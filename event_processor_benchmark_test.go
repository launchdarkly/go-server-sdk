package ldclient

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
)

// These benchmarks cover the DefaultEventProcessor logic for digesting analytics event inputs and producing
// output event data, but not actually sending the event data anywhere.
//
// In the current implementation, event processor tasks are divided between several goroutines. Therefore,
// timing of these operations will have more variability than other benchmarks. However, execution time
// should still be roughly proportional to the volume of work, and allocations should be fairly consistent.

type mockEventSender struct {
	sentCh chan struct{}
}

func (m *mockEventSender) SendEventData(kind ldevents.EventDataKind, data []byte, eventCount int) ldevents.EventSenderResult {
	m.sentCh <- struct{}{} // allows benchmarks to detect that the event payload has been generated and fake-sent
	return ldevents.EventSenderResult{Success: true}
}

type eventsBenchmarkEnv struct {
	eventProcessor   ldevents.EventProcessor
	mockEventSender  *mockEventSender
	targetFeatureKey string
	users            []ldcontext.Context
	variations       []ldvalue.Value
}

func newEventsBenchmarkEnv() *eventsBenchmarkEnv {
	return &eventsBenchmarkEnv{}
}

func (env *eventsBenchmarkEnv) setUp(bc eventsBenchmarkCase) {
	env.mockEventSender = &mockEventSender{sentCh: make(chan struct{}, 10)}

	config := ldevents.EventsConfiguration{
		Capacity:    bc.bufferSize,
		EventSender: env.mockEventSender,
	}
	env.eventProcessor = ldevents.NewDefaultEventProcessor(config)

	env.targetFeatureKey = "flag-key"

	env.variations = make([]ldvalue.Value, bc.numVariations)
	for i := 0; i < bc.numVariations; i++ {
		env.variations[i] = ldvalue.Int(i)
	}

	env.users = make([]ldcontext.Context, bc.numUsers)
	for i := 0; i < bc.numUsers; i++ {
		env.users[i] = lduser.NewUser(makeEvalBenchmarkTargetUserKey(i))
	}
}

func (env *eventsBenchmarkEnv) tearDown() {
	env.eventProcessor.Close()
	env.eventProcessor = nil
}

func (env *eventsBenchmarkEnv) waitUntilEventsSent() {
	<-env.mockEventSender.sentCh
}

type eventsBenchmarkCase struct {
	bufferSize    int
	numEvents     int
	numVariations int
	numUsers      int
}

var eventsBenchmarkCases = []eventsBenchmarkCase{
	{
		bufferSize:    1000,
		numEvents:     100,
		numVariations: 2,
		numUsers:      10,
	},
	{
		bufferSize:    1000,
		numEvents:     100,
		numVariations: 2,
		numUsers:      100,
	},
	{
		bufferSize:    1000,
		numEvents:     1000,
		numVariations: 2,
		numUsers:      10,
	},
	{
		bufferSize:    1000,
		numEvents:     1000,
		numVariations: 2,
		numUsers:      100,
	},
}

func benchmarkEvents(b *testing.B, cases []eventsBenchmarkCase, action func(*eventsBenchmarkEnv, eventsBenchmarkCase)) {
	env := newEventsBenchmarkEnv()
	for _, bc := range cases {
		env.setUp(bc)

		b.Run(fmt.Sprintf("%+v", bc), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				action(env, bc)
			}
		})
		env.tearDown()
	}
}

func BenchmarkFeatureRequestEventsSummaryOnly(b *testing.B) {
	benchmarkEvents(b, eventsBenchmarkCases, func(env *eventsBenchmarkEnv, bc eventsBenchmarkCase) {
		for i := 0; i < bc.numEvents; i++ {
			user := env.users[i%bc.numUsers]
			variation := i % bc.numVariations
			value := env.variations[variation]
			event := ldevents.EvaluationData{
				BaseEvent: ldevents.BaseEvent{
					CreationDate: ldtime.UnixMillisNow(),
					Context:      ldevents.Context(user),
				},
				Key:       env.targetFeatureKey,
				Variation: ldvalue.NewOptionalInt(variation),
				Value:     value,
			}
			env.eventProcessor.RecordEvaluation(event)
		}
		env.eventProcessor.Flush()
		env.waitUntilEventsSent()
	})
}

func BenchmarkFeatureRequestEventsWithFullTracking(b *testing.B) {
	benchmarkEvents(b, eventsBenchmarkCases, func(env *eventsBenchmarkEnv, bc eventsBenchmarkCase) {
		for i := 0; i < bc.numEvents; i++ {
			user := env.users[i%bc.numUsers]
			variation := i % bc.numVariations
			value := env.variations[variation]
			event := ldevents.EvaluationData{
				BaseEvent: ldevents.BaseEvent{
					CreationDate: ldtime.UnixMillisNow(),
					Context:      ldevents.Context(user),
				},
				Key:              env.targetFeatureKey,
				Variation:        ldvalue.NewOptionalInt(variation),
				Value:            value,
				RequireFullEvent: true,
			}
			env.eventProcessor.RecordEvaluation(event)
		}
		env.eventProcessor.Flush()
		env.waitUntilEventsSent()
	})
}

func BenchmarkCustomEvents(b *testing.B) {
	data := ldvalue.ObjectBuild().SetString("eventData", "value").Build()
	benchmarkEvents(b, eventsBenchmarkCases, func(env *eventsBenchmarkEnv, bc eventsBenchmarkCase) {
		for i := 0; i < bc.numEvents; i++ {
			user := env.users[i%bc.numUsers]
			event := ldevents.CustomEventData{
				BaseEvent: ldevents.BaseEvent{
					CreationDate: ldtime.UnixMillisNow(),
					Context:      ldevents.Context(user),
				},
				Key:  "event-key",
				Data: data,
			}
			env.eventProcessor.RecordCustomEvent(event)
		}
		env.eventProcessor.Flush()
		env.waitUntilEventsSent()
	})
}
