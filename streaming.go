package ldclient

import (
	"encoding/json"
	"errors"
	es "github.com/launchdarkly/eventsource"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	putEvent    = "put"
	patchEvent  = "patch"
	deleteEvent = "delete"
)

type streamProcessor struct {
	store        FeatureStore
	stream       *es.Stream
	config       Config
	disconnected *time.Time
	apiKey       string
	ignition     sync.Once
	sync.RWMutex
}

type featurePatchData struct {
	Path string  `json:"path"`
	Data Feature `json:"data"`
}

type featureDeleteData struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
}

func (sp *streamProcessor) Initialized() bool {
	return sp.store.Initialized()
}

func (sp *streamProcessor) GetFeature(key string) (*Feature, error) {
	if !sp.store.Initialized() {
		return nil, errors.New("Requested stream data before initialization completed")
	} else {
		return sp.store.Get(key)
	}
}

func (sp *streamProcessor) ShouldFallbackUpdate() bool {
	sp.RLock()
	defer sp.RUnlock()
	return sp.disconnected != nil && sp.disconnected.Before(time.Now().Add(-2*time.Minute))
}

func (sp *streamProcessor) StartOnce() {
	sp.ignition.Do(func() {
		if !sp.config.UseLdd {
			go sp.start()
			go sp.errors()
		}
	})
}

func (sp *streamProcessor) start() {
	for {
		subscribed := sp.checkSubscribe()
		if !subscribed {
			sp.setDisconnected()
			time.Sleep(2 * time.Second)
			continue
		}
		event := <-sp.stream.Events
		switch event.Event() {
		case putEvent:
			var features map[string]*Feature
			if err := json.Unmarshal([]byte(event.Data()), &features); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature json: %+v", err)
			} else {
				sp.store.Init(features)
				sp.setConnected()
			}
		case patchEvent:
			var patch featurePatchData
			if err := json.Unmarshal([]byte(event.Data()), &patch); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature patch json: %+v", err)
			} else {
				key := strings.TrimLeft(patch.Path, "/")
				sp.store.Upsert(key, patch.Data)
				sp.setConnected()
			}
		case deleteEvent:
			var data featureDeleteData
			if err := json.Unmarshal([]byte(event.Data()), &data); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature delete json: %+v", err)
			} else {
				key := strings.TrimLeft(data.Path, "/")
				sp.store.Delete(key, data.Version)
				sp.setConnected()
			}
		default:
			sp.config.Logger.Printf("Unexpected event found in stream: %s", event.Event())
			sp.setConnected()
		}
	}
}

func newStream(apiKey string, config Config) *streamProcessor {
	var store FeatureStore

	if config.FeatureStore != nil {
		store = config.FeatureStore
	} else {
		store = NewInMemoryFeatureStore()
	}

	sp := &streamProcessor{
		store:  store,
		config: config,
		apiKey: apiKey,
	}

	return sp
}

func (sp *streamProcessor) subscribe() {
	sp.Lock()
	defer sp.Unlock()

	if sp.stream == nil {
		headers := make(http.Header)

		headers.Add("Authorization", "api_key "+sp.apiKey)
		headers.Add("User-Agent", "GoClient/"+Version)

		if stream, err := es.Subscribe(sp.config.StreamUri+"/features", headers, ""); err != nil {
			sp.config.Logger.Printf("Error subscribing to stream: %+v", err)
		} else {
			sp.stream = stream
		}
	}
}

func (sp *streamProcessor) checkSubscribe() bool {
	sp.RLock()
	if sp.stream == nil {
		sp.RUnlock()
		sp.subscribe()
		return sp.stream != nil
	} else {
		defer sp.RUnlock()
		return true
	}
}

func (sp *streamProcessor) errors() {
	for {
		subscribed := sp.checkSubscribe()
		if !subscribed {
			sp.setDisconnected()
			time.Sleep(2 * time.Second)
			continue
		}
		err := <-sp.stream.Errors

		if err != io.EOF {
			sp.config.Logger.Printf("Error encountered processing stream: %+v", err)
		}
		if err != nil {
			sp.setDisconnected()
		}
	}
}

func (sp *streamProcessor) setConnected() {
	sp.RLock()
	if sp.disconnected != nil {
		sp.RUnlock()
		sp.Lock()
		defer sp.Unlock()
		if sp.disconnected != nil {
			sp.disconnected = nil
		}
	} else {
		sp.RUnlock()
	}

}

func (sp *streamProcessor) setDisconnected() {
	sp.RLock()
	if sp.disconnected == nil {
		sp.RUnlock()
		sp.Lock()
		defer sp.Unlock()
		if sp.disconnected == nil {
			now := time.Now()
			sp.disconnected = &now
		}
	} else {
		sp.RUnlock()
	}
}
