package ldclient

import (
	"fmt"
	"testing"
)

// These benchmarks cover the DefaultEventProcessor logic for digesting analytics event inputs and producing
// output event data, but not actually sending the event data anywhere.
//
// In the current implementation, event processor tasks are divided between several goroutines. Therefore,
// timing of these operations will have more variability than other benchmarks. However, execution time
// should still be roughly proportional to the volume of work, and allocations should be fairly consistent.

type eventsBenchmarkEnv struct {
	eventProcessor   *defaultEventProcessor
	targetFeatureKey string
	users            []User
	variations       []interface{}
}

func newEventsBenchmarkEnv() *eventsBenchmarkEnv {
	return &eventsBenchmarkEnv{}
}

func (env *eventsBenchmarkEnv) setUp(bc eventsBenchmarkCase) {
	config := DefaultConfig
	config.Capacity = bc.bufferSize
	env.eventProcessor = newDefaultEventProcessorInternal(
		"sdk-key",
		config,
		nil,
		true, // disableSend == true: event output will be generated, but no HTTP requests will be made
	).(*defaultEventProcessor)

	env.targetFeatureKey = "flag-key"

	env.variations = make([]interface{}, bc.numVariations)
	for i := 0; i < bc.numVariations; i++ {
		env.variations[i] = i
	}

	env.users = make([]User, bc.numUsers)
	for i := 0; i < bc.numUsers; i++ {
		env.users[i] = NewUser(makeEvalBenchmarkTargetUserKey(i))
	}
}

func (env *eventsBenchmarkEnv) tearDown() {
	env.eventProcessor.Close()
	env.eventProcessor = nil
}

type eventsBenchmarkCase struct {
	bufferSize    int
	numEvents     int
	numVariations int
	numUsers      int
}

var eventsBenchmarkCases = []eventsBenchmarkCase{
	eventsBenchmarkCase{
		bufferSize:    1000,
		numEvents:     100,
		numVariations: 2,
		numUsers:      10,
	},
	eventsBenchmarkCase{
		bufferSize:    1000,
		numEvents:     100,
		numVariations: 2,
		numUsers:      100,
	},
	eventsBenchmarkCase{
		bufferSize:    1000,
		numEvents:     1000,
		numVariations: 2,
		numUsers:      10,
	},
	eventsBenchmarkCase{
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
			event := FeatureRequestEvent{
				BaseEvent: BaseEvent{
					CreationDate: now(),
					User:         user,
				},
				Key:       env.targetFeatureKey,
				Variation: &variation,
				Value:     value,
				Default:   nil,
				PrereqOf:  nil,
			}
			env.eventProcessor.SendEvent(event)
		}
		env.eventProcessor.Flush()
		env.eventProcessor.waitUntilInactive()
	})
}

func BenchmarkFeatureRequestEventsWithFullTracking(b *testing.B) {
	benchmarkEvents(b, eventsBenchmarkCases, func(env *eventsBenchmarkEnv, bc eventsBenchmarkCase) {
		for i := 0; i < bc.numEvents; i++ {
			user := env.users[i%bc.numUsers]
			variation := i % bc.numVariations
			value := env.variations[variation]
			event := FeatureRequestEvent{
				BaseEvent: BaseEvent{
					CreationDate: now(),
					User:         user,
				},
				Key:         env.targetFeatureKey,
				Variation:   &variation,
				Value:       value,
				Default:     nil,
				PrereqOf:    nil,
				TrackEvents: true,
			}
			env.eventProcessor.SendEvent(event)
		}
		env.eventProcessor.Flush()
		env.eventProcessor.waitUntilInactive()
	})
}

func BenchmarkCustomEvents(b *testing.B) {
	data := map[string]interface{}{"eventData": "value"}
	benchmarkEvents(b, eventsBenchmarkCases, func(env *eventsBenchmarkEnv, bc eventsBenchmarkCase) {
		for i := 0; i < bc.numEvents; i++ {
			user := env.users[i%bc.numUsers]
			event := CustomEvent{
				BaseEvent: BaseEvent{
					CreationDate: now(),
					User:         user,
				},
				Key:  "event-key",
				Data: data,
			}
			env.eventProcessor.SendEvent(event)
		}
		env.eventProcessor.Flush()
		env.eventProcessor.waitUntilInactive()
	})
}
