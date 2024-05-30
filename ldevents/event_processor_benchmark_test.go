package ldevents

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const benchmarkEventCount = 100

// Timings for BenchmarkEventProcessor are likely to vary a lot, because it is testing the full pipeline of
// event processing from EventProcessor.Send() until the payload is handed off to the EventSender, which
// takes place across multiple goroutines. We are mostly concerned with allocations, and general trends in
// execution time.

func BenchmarkEventProcessor(b *testing.B) {
	configDefault := EventsConfiguration{Capacity: 1000}

	doEvents := func(b *testing.B, config EventsConfiguration, sendEvents func(EventProcessor)) {
		ep, es := createBenchmarkEventProcessorAndSender(config)
		defer ep.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			sendEvents(ep)
			ep.Flush()
			es.awaitPayload()
		}

		b.StopTimer() // so ep.Close() isn't included in the time
	}

	b.Run("summarize feature events", func(b *testing.B) {
		doEvents(b, configDefault, sendBenchmarkFeatureEvents(false))
	})

	b.Run("feature events with full tracking", func(b *testing.B) {
		doEvents(b, configDefault, sendBenchmarkFeatureEvents(true))
	})

	b.Run("custom events", func(b *testing.B) {
		doEvents(b, configDefault, sendBenchmarkCustomEvents())
	})
}

func makeBenchmarkUsers() []ldcontext.Context {
	numUsers := 10
	ret := make([]ldcontext.Context, 0, numUsers)
	for i := 0; i < numUsers; i++ {
		user := lduser.NewUserBuilder(fmt.Sprintf("user%d", i)).
			Name(fmt.Sprintf("name%d", i)).
			Build()
		ret = append(ret, user)
	}
	return ret
}

func sendBenchmarkFeatureEvents(tracking bool) func(EventProcessor) {
	events := make([]EvaluationData, 0, benchmarkEventCount)
	users := makeBenchmarkUsers()
	flagCount := 10
	flagVersions := 3
	flagVariations := 2
	rnd := rand.New(rand.NewSource(int64(ldtime.UnixMillisNow())))

	for i := 0; i < benchmarkEventCount; i++ {
		variation := rnd.Intn(flagVariations)
		event := EvaluationData{
			BaseEvent: BaseEvent{
				Context:      Context(users[rnd.Intn(len(users))]),
				CreationDate: ldtime.UnixMillisNow(),
			},
			Key:              fmt.Sprintf("flag%d", rnd.Intn(flagCount)),
			Version:          ldvalue.NewOptionalInt(rnd.Intn(flagVersions) + 1),
			Variation:        ldvalue.NewOptionalInt(variation),
			Value:            ldvalue.Int(variation),
			RequireFullEvent: tracking,
		}
		events = append(events, event)
	}

	return func(ep EventProcessor) {
		for _, e := range events {
			ep.RecordEvaluation(e)
		}
	}
}

func sendBenchmarkCustomEvents() func(EventProcessor) {
	events := make([]CustomEventData, 0, benchmarkEventCount)
	users := makeBenchmarkUsers()
	keyCount := 5
	rnd := rand.New(rand.NewSource(int64(ldtime.UnixMillisNow())))

	for i := 0; i < benchmarkEventCount; i++ {
		event := CustomEventData{
			BaseEvent: BaseEvent{
				Context:      Context(users[rnd.Intn(len(users))]),
				CreationDate: ldtime.UnixMillisNow(),
			},
			Key:  fmt.Sprintf("event%d", rnd.Intn(keyCount)),
			Data: ldvalue.String("data"),
		}
		events = append(events, event)
	}

	return func(ep EventProcessor) {
		for _, e := range events {
			ep.RecordCustomEvent(e)
		}
	}
}

// This is  simpler than the mockEventSender used in other tests, because we don't need to parse the event
// payload - we just want to know when it's been sent - and we don't need to simulate different results.
type benchmarkMockEventSender struct {
	payloadCh chan []byte
}

func newBenchmarkMockEventSender() *benchmarkMockEventSender {
	return &benchmarkMockEventSender{
		payloadCh: make(chan []byte, 100),
	}
}

func (ms *benchmarkMockEventSender) SendEventData(
	kind EventDataKind,
	data []byte,
	eventCount int,
) EventSenderResult {
	ms.payloadCh <- data
	return EventSenderResult{Success: true}
}

func (ms *benchmarkMockEventSender) awaitPayload() {
	<-ms.payloadCh
}

func createBenchmarkEventProcessorAndSender(config EventsConfiguration) (EventProcessor, *benchmarkMockEventSender) {
	sender := newBenchmarkMockEventSender()
	config.EventSender = sender
	return NewDefaultEventProcessor(config), sender
}
