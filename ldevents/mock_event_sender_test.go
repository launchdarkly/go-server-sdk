package ldevents

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockEventSender struct {
	events             []json.RawMessage
	diagnosticEvents   []json.RawMessage
	eventsCh           chan json.RawMessage
	diagnosticEventsCh chan json.RawMessage
	payloadCount       int
	result             EventSenderResult
	gateCh             <-chan struct{}
	waitingCh          chan<- struct{}
	lock               sync.Mutex
}

func newMockEventSender() *mockEventSender {
	return &mockEventSender{
		eventsCh:           make(chan json.RawMessage, 100),
		diagnosticEventsCh: make(chan json.RawMessage, 100),
		result:             EventSenderResult{Success: true},
	}
}

func (ms *mockEventSender) SendEventData(kind EventDataKind, data []byte, eventCount int) EventSenderResult {
	ms.lock.Lock()
	if kind == DiagnosticEventDataKind {
		ms.diagnosticEvents = append(ms.diagnosticEvents, data)
		ms.diagnosticEventsCh <- data
	} else {
		var dataAsArray []json.RawMessage
		if err := json.Unmarshal(data, &dataAsArray); err != nil {
			panic(err)
		}
		for _, elementData := range dataAsArray {
			ms.events = append(ms.events, elementData)
			ms.eventsCh <- elementData
		}
		ms.payloadCount++
	}
	gateCh, waitingCh := ms.gateCh, ms.waitingCh
	result := ms.result
	ms.lock.Unlock()

	if gateCh != nil {
		// instrumentation used for TestBlockingFlush and TestEventsAreKeptInBufferIfAllFlushWorkersAreBusy
		waitingCh <- struct{}{}
		<-gateCh
	}

	return result
}

func (ms *mockEventSender) setGate(gateCh <-chan struct{}, waitingCh chan<- struct{}) {
	ms.lock.Lock()
	ms.gateCh = gateCh
	ms.waitingCh = waitingCh
	ms.lock.Unlock()
}

func (ms *mockEventSender) getPayloadCount() int {
	ms.lock.Lock()
	defer ms.lock.Unlock()
	return ms.payloadCount
}

func (ms *mockEventSender) awaitEvent(t *testing.T) json.RawMessage {
	event, ok := ms.tryAwaitEvent()
	if !ok {
		require.Fail(t, "timed out waiting for analytics event")
	}
	return event
}

func (ms *mockEventSender) tryAwaitEvent() (json.RawMessage, bool) {
	return ms.tryAwaitEventCh(ms.eventsCh)
}

func (ms *mockEventSender) awaitDiagnosticEvent(t *testing.T) json.RawMessage {
	event, ok := ms.tryAwaitEventCh(ms.diagnosticEventsCh)
	if !ok {
		require.Fail(t, "timed out waiting for diagnostic event")
	}
	return event
}

func (ms *mockEventSender) tryAwaitEventCh(ch <-chan json.RawMessage) (json.RawMessage, bool) {
	select {
	case e := <-ch:
		return e, true
	case <-time.After(time.Second):
		break
	}
	return nil, false
}

func (ms *mockEventSender) assertNoMoreEvents(t *testing.T) {
	require.Len(t, ms.eventsCh, 0)
}

func (ms *mockEventSender) assertNoMoreDiagnosticEvents(t *testing.T) {
	require.Len(t, ms.diagnosticEventsCh, 0)
}
