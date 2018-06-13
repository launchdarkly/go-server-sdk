package ldclient

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"errors"

	es "github.com/launchdarkly/eventsource"
)

const (
	putEvent           = "put"
	patchEvent         = "patch"
	deleteEvent        = "delete"
	indirectPatchEvent = "indirect/patch"
)

type streamProcessor struct {
	store              FeatureStore
	requestor          *requestor
	stream             *es.Stream
	config             Config
	sdkKey             string
	setInitializedOnce sync.Once
	isInitialized      bool
	halt               chan struct{}
	closeOnce          sync.Once
}

type putData struct {
	Path string  `json:"path"`
	Data allData `json:"data"`
}

type allData struct {
	Flags    map[string]*FeatureFlag `json:"flags"`
	Segments map[string]*Segment     `json:"segments"`
}

type patchData struct {
	Path string `json:"path"`
	// This could be a flag or a segment, or something else, depending on the path
	Data json.RawMessage `json:"data"`
}

type deleteData struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
}

func (sp *streamProcessor) Initialized() bool {
	return sp.isInitialized
}

func (sp *streamProcessor) Start(closeWhenReady chan<- struct{}) {
	sp.config.Logger.Printf("Starting LaunchDarkly streaming connection")
	go sp.subscribe(closeWhenReady)
}

func flagKey(path string) (string, error) {
	if strings.HasPrefix(path, "/flags/") {
		return strings.TrimPrefix(path, "/flags/"), nil
	}

	return "", errors.New("Not a flag path")
}

func segmentKey(path string) (string, error) {
	if strings.HasPrefix(path, "/segments/") {
		return strings.TrimPrefix(path, "/segments/"), nil
	}

	return "", errors.New("Not a segment path")
}

func (sp *streamProcessor) events(closeWhenReady chan<- struct{}) {
	var readyOnce sync.Once
	notifyReady := func() {
		readyOnce.Do(func() {
			close(closeWhenReady)
		})
	}
	// Ensure we stop waiting for initialization if we exit, even if initialization fails
	defer notifyReady()

	for {
		select {
		case event, ok := <-sp.stream.Events:
			if !ok {
				sp.config.Logger.Printf("Event stream closed.")
				return
			}
			switch event.Event() {
			case putEvent:
				var put putData
				if err := json.Unmarshal([]byte(event.Data()), &put); err != nil {
					sp.config.Logger.Printf("ERROR: Unexpected error unmarshalling PUT json: %+v", err)
				} else {
					err = sp.store.Init(MakeAllVersionedDataMap(put.Data.Flags, put.Data.Segments))
					if err != nil {
						sp.config.Logger.Printf("Error initializing store: %s", err)
						return
					}
					sp.setInitializedOnce.Do(func() {
						sp.config.Logger.Printf("Started LaunchDarkly streaming client")
						sp.isInitialized = true
						notifyReady()
					})
				}
			case patchEvent:
				var patch patchData
				if err := json.Unmarshal([]byte(event.Data()), &patch); err != nil {
					sp.config.Logger.Printf("ERROR: Unexpected error unmarshalling PATCH json: %+v", err)
				} else {
					if _, err = flagKey(patch.Path); err == nil {
						var flag FeatureFlag
						if err = json.Unmarshal(patch.Data, &flag); err != nil {
							sp.config.Logger.Printf("ERROR: Unexpected error unmarshalling feature flag json: %+v", err)
						} else {
							if err = sp.store.Upsert(Features, &flag); err != nil {
								sp.config.Logger.Printf("ERROR: Unexpected error storing feature flag json: %+v", err)
							}
						}
					} else if _, err = segmentKey(patch.Path); err == nil {
						var segment Segment
						if err = json.Unmarshal(patch.Data, &segment); err != nil {
							sp.config.Logger.Printf("ERROR: Unexpected error unmarshalling segment json: %+v", err)
						} else {
							if err = sp.store.Upsert(Segments, &segment); err != nil {
								sp.config.Logger.Printf("ERROR: Unexpected error storing segment json: %+v", err)
							}
						}
					} else {
						sp.config.Logger.Printf("ERROR: Unknown data path: %s. Ignoring patch.", patch.Path)
					}
				}
			case deleteEvent:
				var data deleteData
				if err := json.Unmarshal([]byte(event.Data()), &data); err != nil {
					sp.config.Logger.Printf("ERROR: Unexpected error unmarshalling DELETE json: %+v", err)
				}
				if key, err := flagKey(data.Path); err == nil {
					if err = sp.store.Delete(Features, key, data.Version); err != nil {
						sp.config.Logger.Printf(`ERROR: Unexpected error deleting feature flag "%s"`, key, err)
					}
				} else if key, err = segmentKey(data.Path); err == nil {
					if err = sp.store.Delete(Segments, key, data.Version); err != nil {
						sp.config.Logger.Printf(`ERROR: Unexpected error deleting segment "%s"`, key, err)
					}
				} else {
					sp.config.Logger.Printf("ERROR: Unknown data path: %s. Ignoring delete.", data.Path)
				}
			case indirectPatchEvent:
				path := event.Data()
				if key, err := flagKey(path); err == nil {
					if feature, requestErr := sp.requestor.requestFlag(key); requestErr != nil {
						sp.config.Logger.Printf(`ERROR: Unexpected error requesting feature "%s": %+v`, key, requestErr)
					} else {
						if err = sp.store.Upsert(Features, feature); err != nil {
							sp.config.Logger.Printf(`ERROR: Unexpected error requesting feature "%s": %+v`, key, requestErr)
						}
					}
				} else if key, err = segmentKey(path); err == nil {
					if segment, requestErr := sp.requestor.requestSegment(key); requestErr != nil {
						sp.config.Logger.Printf(`ERROR: Unexpected error requesting segment "%s": %+v`, segment, requestErr)
					} else {
						if err = sp.store.Upsert(Segments, segment); err != nil {
							sp.config.Logger.Printf(`ERROR: Unexpected error requesting segment "%s": %+v`, segment, requestErr)
						}
					}
				}
			default:
				sp.config.Logger.Printf("Unexpected event found in stream: %s", event.Event())
			}
		case err, ok := <-sp.stream.Errors:
			if !ok {
				sp.config.Logger.Printf("Event error stream closed.")
			}
			if err != io.EOF {
				sp.config.Logger.Printf("ERROR: Error encountered processing stream: %+v", err)
				if sp.checkUnauthorized(err) {
					sp.closeOnce.Do(func() {
						sp.config.Logger.Printf("Closing event stream.")
						// TODO: enable this when we trust stream.Close() never to panic (see https://github.com/donovanhide/eventsource/pull/33)
						// Until we're able to Close it explicitly here, we won't be able to stop it from trying to reconnect after a 401 error.
						// sp.stream.Close()
					})
					return
				}
			}
		case <-sp.halt:
			return
		}
	}
}

func newStreamProcessor(sdkKey string, config Config, requestor *requestor) *streamProcessor {
	sp := &streamProcessor{
		store:     config.FeatureStore,
		config:    config,
		sdkKey:    sdkKey,
		requestor: requestor,
		halt:      make(chan struct{}),
	}

	return sp
}

func (sp *streamProcessor) subscribe(closeWhenReady chan<- struct{}) {
	for {
		req, _ := http.NewRequest("GET", sp.config.StreamUri+"/all", nil)
		req.Header.Add("Authorization", sp.sdkKey)
		req.Header.Add("User-Agent", sp.config.UserAgent)
		sp.config.Logger.Printf("Connecting to LaunchDarkly stream using URL: %s", req.URL.String())

		if stream, err := es.SubscribeWithRequest("", req); err != nil {
			sp.config.Logger.Printf("ERROR: Error subscribing to stream: %+v using URL: %s", err, req.URL.String())
			if sp.checkUnauthorized(err) {
				close(closeWhenReady)
				return
			}

			// Halt immediately if we've been closed already
			select {
			case <-sp.halt:
				close(closeWhenReady)
				return
			default:
				time.Sleep(2 * time.Second)
			}
		} else {
			sp.stream = stream
			sp.stream.Logger = sp.config.Logger

			go sp.events(closeWhenReady)
			return
		}
	}
}

func (sp *streamProcessor) checkUnauthorized(err error) bool {
	if se, ok := err.(es.SubscriptionError); ok {
		if se.Code == 401 {
			sp.config.Logger.Printf("ERROR: Received 401 error, no further streaming connection will be made since SDK key is invalid")
			return true
		}
	}
	return false
}

// Close instructs the processor to stop receiving updates
func (sp *streamProcessor) Close() error {
	sp.closeOnce.Do(func() {
		sp.config.Logger.Printf("Closing event stream.")
		sp.stream.Close()
		close(sp.halt)
	})
	return nil
}
