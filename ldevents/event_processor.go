package ldevents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

type defaultEventProcessor struct {
	inboxCh       chan eventDispatcherMessage
	inboxFullOnce sync.Once
	closeOnce     sync.Once
	loggers       ldlog.Loggers
}

type eventDispatcher struct {
	config            EventsConfiguration
	lastKnownPastTime uint64
	deduplicatedUsers int
	eventsInLastBatch int
	disabled          bool
	stateLock         sync.Mutex
}

type flushPayload struct {
	diagnosticEvent interface{}
	events          []Event
	summary         eventSummary
}

type sendEventsTask struct {
	client    *http.Client
	config    EventsConfiguration
	formatter eventOutputFormatter
}

// Payload of the inboxCh channel.
type eventDispatcherMessage interface{}

type sendEventMessage struct {
	event Event
}

type flushEventsMessage struct{}

type shutdownEventsMessage struct {
	replyCh chan struct{}
}

type syncEventsMessage struct {
	replyCh chan struct{}
}

const (
	maxFlushWorkers    = 5
	eventSchemaHeader  = "X-LaunchDarkly-Event-Schema"
	payloadIDHeader    = "X-LaunchDarkly-Payload-ID"
	currentEventSchema = "3"
	defaultURIPath     = "/bulk"
	diagnosticsURIPath = "/diagnostic"
)

// NewDefaultEventProcessor creates an instance of the default implementation of analytics event processing.
func NewDefaultEventProcessor(config EventsConfiguration) EventProcessor {
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	inboxCh := make(chan eventDispatcherMessage, config.Capacity)
	startEventDispatcher(config, inboxCh)
	return &defaultEventProcessor{
		inboxCh: inboxCh,
		loggers: config.Loggers,
	}
}

func (ep *defaultEventProcessor) SendEvent(e Event) {
	ep.postNonBlockingMessageToInbox(sendEventMessage{event: e})
}

func (ep *defaultEventProcessor) Flush() {
	ep.postNonBlockingMessageToInbox(flushEventsMessage{})
}

func (ep *defaultEventProcessor) postNonBlockingMessageToInbox(e eventDispatcherMessage) bool {
	select {
	case ep.inboxCh <- e:
		return true
	default:
	}
	// If the inbox is full, it means the eventDispatcher is seriously backed up with not-yet-processed events.
	// This is unlikely, but if it happens, it means the application is probably doing a ton of flag evaluations
	// across many goroutines-- so if we wait for a space in the inbox, we risk a very serious slowdown of the
	// app. To avoid that, we'll just drop the event. The log warning about this will only be shown once.
	ep.inboxFullOnce.Do(func() {
		ep.loggers.Warn("Events are being produced faster than they can be processed; some events will be dropped")
	})
	return false
}

func (ep *defaultEventProcessor) Close() error {
	ep.closeOnce.Do(func() {
		// We put the flush and shutdown messages directly into the channel instead of calling
		// postNonBlockingMessageToInbox, because we *do* want to block to make sure there is room in the channel;
		// these aren't analytics events, they are messages that are necessary for an orderly shutdown.
		ep.inboxCh <- flushEventsMessage{}
		m := shutdownEventsMessage{replyCh: make(chan struct{})}
		ep.inboxCh <- m
		<-m.replyCh
	})
	return nil
}

func startEventDispatcher(
	config EventsConfiguration,
	inboxCh <-chan eventDispatcherMessage,
) {
	ed := &eventDispatcher{
		config: config,
	}

	// Start a fixed-size pool of workers that wait on flushTriggerCh. This is the
	// maximum number of flushes we can do concurrently.
	flushCh := make(chan *flushPayload, 1)
	var workersGroup sync.WaitGroup
	for i := 0; i < maxFlushWorkers; i++ {
		startFlushTask(config, flushCh, &workersGroup,
			func(r *http.Response) { ed.handleResponse(r) })
	}
	if config.DiagnosticsManager != nil {
		event := config.DiagnosticsManager.CreateInitEvent()
		ed.sendDiagnosticsEvent(event, flushCh, &workersGroup)
	}
	go ed.runMainLoop(inboxCh, flushCh, &workersGroup)
}

func (ed *eventDispatcher) runMainLoop(
	inboxCh <-chan eventDispatcherMessage,
	flushCh chan<- *flushPayload,
	workersGroup *sync.WaitGroup,
) {
	if err := recover(); err != nil {
		ed.config.Loggers.Errorf("Unexpected panic in event processing thread: %+v", err)
	}

	outbox := newEventsOutbox(ed.config.Capacity, ed.config.Loggers)

	userKeys := newLruCache(ed.config.UserKeysCapacity)

	flushInterval := ed.config.FlushInterval
	if flushInterval <= 0 {
		flushInterval = DefaultFlushInterval
	}
	userKeysFlushInterval := ed.config.UserKeysFlushInterval
	if userKeysFlushInterval <= 0 {
		userKeysFlushInterval = DefaultUserKeysFlushInterval
	}
	flushTicker := time.NewTicker(flushInterval)
	usersResetTicker := time.NewTicker(userKeysFlushInterval)

	var diagnosticsTicker *time.Ticker
	var diagnosticsTickerCh <-chan time.Time
	diagnosticsManager := ed.config.DiagnosticsManager
	if diagnosticsManager != nil {
		interval := ed.config.DiagnosticRecordingInterval
		if interval <= 0 {
			interval = DefaultDiagnosticRecordingInterval
		}
		diagnosticsTicker = time.NewTicker(interval)
		diagnosticsTickerCh = diagnosticsTicker.C
	}

	for {
		// Drain the response channel with a higher priority than anything else
		// to ensure that the flush workers don't get blocked.
		select {
		case message := <-inboxCh:
			switch m := message.(type) {
			case sendEventMessage:
				ed.processEvent(m.event, outbox, &userKeys)
			case flushEventsMessage:
				ed.triggerFlush(outbox, flushCh, workersGroup)
			case syncEventsMessage:
				workersGroup.Wait()
				m.replyCh <- struct{}{}
			case shutdownEventsMessage:
				flushTicker.Stop()
				usersResetTicker.Stop()
				if diagnosticsTicker != nil {
					diagnosticsTicker.Stop()
				}
				workersGroup.Wait() // Wait for all in-progress flushes to complete
				close(flushCh)      // Causes all idle flush workers to terminate
				m.replyCh <- struct{}{}
				return
			}
		case <-flushTicker.C:
			ed.triggerFlush(outbox, flushCh, workersGroup)
		case <-usersResetTicker.C:
			userKeys.clear()
		case <-diagnosticsTickerCh:
			if diagnosticsManager == nil || !diagnosticsManager.CanSendStatsEvent() {
				break
			}
			event := diagnosticsManager.CreateStatsEventAndReset(
				outbox.droppedEvents,
				ed.deduplicatedUsers,
				ed.eventsInLastBatch,
			)
			outbox.droppedEvents = 0
			ed.deduplicatedUsers = 0
			ed.eventsInLastBatch = 0
			ed.sendDiagnosticsEvent(event, flushCh, workersGroup)
		}
	}
}

func (ed *eventDispatcher) processEvent(evt Event, outbox *eventsOutbox, userKeys *lruCache) {
	// Always record the event in the summarizer.
	outbox.addToSummary(evt)

	// Decide whether to add the event to the payload. Feature events may be added twice, once for
	// the event (if tracked) and once for debugging.
	willAddFullEvent := false
	var debugEvent Event
	switch evt := evt.(type) {
	case FeatureRequestEvent:
		willAddFullEvent = evt.TrackEvents
		if ed.shouldDebugEvent(&evt) {
			de := evt
			de.Debug = true
			debugEvent = de
		}
	default:
		willAddFullEvent = true
	}

	// For each user we haven't seen before, we add an index event - unless this is already
	// an identify event for that user. This should be added before the event that referenced
	// the user, and can be omitted if that event will contain an inline user.
	if !(willAddFullEvent && ed.config.InlineUsersInEvents) {
		user := evt.GetBase().User
		if noticeUser(userKeys, &user) {
			ed.deduplicatedUsers++
		} else {
			if _, ok := evt.(IdentifyEvent); !ok {
				indexEvent := IndexEvent{
					BaseEvent{CreationDate: evt.GetBase().CreationDate, User: user},
				}
				outbox.addEvent(indexEvent)
			}
		}
	}
	if willAddFullEvent {
		outbox.addEvent(evt)
	}
	if debugEvent != nil {
		outbox.addEvent(debugEvent)
	}
}

// Add to the set of users we've noticed, and return true if the user was already known to us.
func noticeUser(userKeys *lruCache, user *lduser.User) bool {
	if user == nil {
		return true
	}
	return userKeys.add(user.GetKey())
}

func (ed *eventDispatcher) shouldDebugEvent(evt *FeatureRequestEvent) bool {
	if evt.DebugEventsUntilDate == nil {
		return false
	}
	// The "last known past time" comes from the last HTTP response we got from the server.
	// In case the client's time is set wrong, at least we know that any expiration date
	// earlier than that point is definitely in the past.  If there's any discrepancy, we
	// want to err on the side of cutting off event debugging sooner.
	ed.stateLock.Lock() // This should be done infrequently since it's only for debug events
	defer ed.stateLock.Unlock()
	return *evt.DebugEventsUntilDate > ed.lastKnownPastTime &&
		*evt.DebugEventsUntilDate > now()
}

// Signal that we would like to do a flush as soon as possible.
func (ed *eventDispatcher) triggerFlush(outbox *eventsOutbox, flushCh chan<- *flushPayload,
	workersGroup *sync.WaitGroup) {
	if ed.isDisabled() {
		outbox.clear()
		return
	}
	// Is there anything to flush?
	payload := outbox.getPayload()
	totalEventCount := len(payload.events)
	if len(payload.summary.counters) > 0 {
		totalEventCount++
	}
	if totalEventCount == 0 {
		ed.eventsInLastBatch = 0
		return
	}
	workersGroup.Add(1) // Increment the count of active flushes
	select {
	case flushCh <- &payload:
		// If the channel wasn't full, then there is a worker available who will pick up
		// this flush payload and send it. The event outbox and summary state can now be
		// cleared from the main goroutine.
		ed.eventsInLastBatch = totalEventCount
		outbox.clear()
	default:
		// We can't start a flush right now because we're waiting for one of the workers
		// to pick up the last one.  Do not reset the event outbox or summary state.
		workersGroup.Done()
	}
}

func (ed *eventDispatcher) isDisabled() bool {
	// Since we're using a mutex, we should avoid calling this often.
	ed.stateLock.Lock()
	defer ed.stateLock.Unlock()
	return ed.disabled
}

func (ed *eventDispatcher) handleResponse(resp *http.Response) {
	if err := checkForHttpError(resp.StatusCode, resp.Request.URL.String()); err != nil {
		ed.config.Loggers.Error(httpErrorMessage(resp.StatusCode, "posting events", "some events were dropped"))
		if !isHTTPErrorRecoverable(resp.StatusCode) {
			ed.stateLock.Lock()
			defer ed.stateLock.Unlock()
			ed.disabled = true
		}
	} else {
		dt, err := http.ParseTime(resp.Header.Get("Date"))
		if err == nil {
			ed.stateLock.Lock()
			defer ed.stateLock.Unlock()
			ed.lastKnownPastTime = toUnixMillis(dt)
		}
	}
}

func (ed *eventDispatcher) sendDiagnosticsEvent(
	event interface{},
	flushCh chan<- *flushPayload,
	workersGroup *sync.WaitGroup,
) {
	payload := flushPayload{diagnosticEvent: event}
	workersGroup.Add(1) // Increment the count of active flushes
	select {
	case flushCh <- &payload:
		// If the channel wasn't full, then there is a worker available who will pick up
		// this flush payload and send it.
	default:
		// We can't start a flush right now because we're waiting for one of the workers
		// to pick up the last one. We'll just discard this diagnostic event - presumably
		// we'll send another one later anyway, and we don't want this kind of nonessential
		// data to cause any kind of back-pressure.
		workersGroup.Done()
	}
}

func startFlushTask(config EventsConfiguration, flushCh <-chan *flushPayload,
	workersGroup *sync.WaitGroup, responseFn func(*http.Response)) {
	ef := eventOutputFormatter{
		userFilter: newUserFilter(config),
		config:     config,
	}
	t := sendEventsTask{
		client:    config.HTTPClient,
		config:    config,
		formatter: ef,
	}
	go t.run(flushCh, responseFn, workersGroup)
}

func (t *sendEventsTask) run(flushCh <-chan *flushPayload, responseFn func(*http.Response),
	workersGroup *sync.WaitGroup) {
	for {
		payload, more := <-flushCh
		if !more {
			// Channel has been closed - we're shutting down
			break
		}
		if payload.diagnosticEvent != nil {
			t.postEvents(t.config.DiagnosticURI, payload.diagnosticEvent, "diagnostic event")
		} else {
			outputEvents := t.formatter.makeOutputEvents(payload.events, payload.summary)
			if len(outputEvents) > 0 {
				resp := t.postEvents(t.config.EventsURI, outputEvents, fmt.Sprintf("%d events", len(outputEvents)))
				if resp != nil {
					responseFn(resp)
				}
			}
		}
		workersGroup.Done() // Decrement the count of in-progress flushes
	}
}

func (t *sendEventsTask) postEvents(uri string, outputData interface{}, description string) *http.Response {
	jsonPayload, marshalErr := json.Marshal(outputData)
	if marshalErr != nil {
		t.config.Loggers.Errorf("Unexpected error marshalling event json: %+v", marshalErr)
		return nil
	}
	payloadUUID, _ := uuid.NewRandom()
	payloadID := payloadUUID.String() // if NewRandom somehow failed, we'll just proceed with an empty string

	t.config.Loggers.Debugf("Sending %s: %s", description, jsonPayload)

	var resp *http.Response
	var respErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			t.config.Loggers.Warn("Will retry posting events after 1 second")
			time.Sleep(1 * time.Second)
		}
		req, reqErr := http.NewRequest("POST", uri, bytes.NewReader(jsonPayload))
		if reqErr != nil {
			t.config.Loggers.Errorf("Unexpected error while creating event request: %+v", reqErr)
			return nil
		}

		for k, vv := range t.config.Headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add(eventSchemaHeader, currentEventSchema)
		req.Header.Add(payloadIDHeader, payloadID)

		resp, respErr = t.client.Do(req)

		if resp != nil && resp.Body != nil {
			_, _ = ioutil.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}

		if respErr != nil {
			t.config.Loggers.Warnf("Unexpected error while sending events: %+v", respErr)
			continue
		} else if resp.StatusCode >= 400 && isHTTPErrorRecoverable(resp.StatusCode) {
			t.config.Loggers.Warnf("Received error status %d when sending events", resp.StatusCode)
			continue
		} else {
			break
		}
	}
	return resp
}
