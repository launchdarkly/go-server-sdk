package ldclient

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	ldevents "github.com/launchdarkly/go-sdk-events/v2"
)

// MigrationOpTracker is used to collect migration related measurements. These measurements will be
// sent upstream to LaunchDarkly servers and used to enhance the visibility of in progress
// migrations.
type MigrationOpTracker struct {
	flagKey            string
	defaultStage       ldmigration.Stage
	op                 *ldmigration.Operation
	samplingRatio      uint32
	context            ldcontext.Context
	evaluation         ldreason.EvaluationDetail // TODO: Placeholder type for now
	consistencyCheck   *ldmigration.ConsistencyCheck
	errors             map[ldmigration.Origin]bool
	latency            map[ldmigration.Origin]int
	customMeasurements map[string]map[ldmigration.Origin]float64

	lock sync.Mutex
}

// NewMigrationOpTracker creates a tracker instance that can be used to capture migration related
// measurement data.
//
// By default, the MigrationOpTracker is invalid. You must set an operation using
// [MigrationOpTracker.Operation] before the tracker can generate valid event date using
// [MigrationOpTracker.Build].
func NewMigrationOpTracker(flagKey string, context ldcontext.Context, detail ldreason.EvaluationDetail, defaultStage ldmigration.Stage) *MigrationOpTracker {
	return &MigrationOpTracker{
		flagKey:            flagKey,
		defaultStage:       defaultStage,
		context:            context,
		evaluation:         detail,
		errors:             make(map[ldmigration.Origin]bool),
		latency:            make(map[ldmigration.Origin]int),
		customMeasurements: make(map[string]map[ldmigration.Origin]float64),
	}
}

// Operation sets the migration related operation associated with these tracking measurements.
func (t *MigrationOpTracker) Operation(op ldmigration.Operation) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.op = &op
}

// TrackConsistency allows recording the results of a consistency check, along with the
// sampling ratio used to collect that information.
func (t *MigrationOpTracker) TrackConsistency(wasConsistent bool, samplingRatio int) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.consistencyCheck = ldmigration.NewConsistencyCheck(wasConsistent, samplingRatio)
}

// TrackError allows recording whether or not an error occurred during the operation.
func (t *MigrationOpTracker) TrackError(origin ldmigration.Origin, hadError bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.errors[origin] = hadError
}

// TrackLatency allows tracking the recorded latency for an individual operation.
func (t *MigrationOpTracker) TrackLatency(origin ldmigration.Origin, duration time.Duration) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.latency[origin] = int(duration.Milliseconds())
}

// TrackCustom allows tracking of custom defined measurements.
func (t *MigrationOpTracker) TrackCustom(key string, origin ldmigration.Origin, value float64) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.customMeasurements[key] == nil {
		t.customMeasurements[key] = make(map[ldmigration.Origin]float64)
	}

	t.customMeasurements[key][origin] = value
}

// Build creates an instance of [ldevents.MigrationOpEventData]. This event data can be provided to
// the [LDClient.TrackMigrationOp] method to rely this metric information upstream to LaunchDarkly
// services.
func (t *MigrationOpTracker) Build() (*ldevents.MigrationOpEventData, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if len(t.flagKey) == 0 {
		return nil, errors.New("migration operation cannot contain an empty flag key")
	}

	if t.op == nil {
		return nil, errors.New("migration operation not specified")
	}

	if err := t.context.Err(); err != nil {
		return nil, fmt.Errorf("invalid context given; %s", err)
	}

	return &ldevents.MigrationOpEventData{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: ldtime.UnixMillisNow(),
			Context:      ldevents.Context(t.context),
		},
		Op:                 *t.op,
		FlagKey:            t.flagKey,
		Default:            t.defaultStage,
		Evaluation:         t.evaluation,
		SamplingRatio:      0, // TODO: Need to deal with this still
		ConsistencyCheck:   t.consistencyCheck,
		Errors:             t.errors,
		Latency:            t.latency,
		CustomMeasurements: t.customMeasurements,
	}, nil
}
