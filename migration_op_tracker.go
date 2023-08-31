package ldclient

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldsampling"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// MigrationOpTracker is used to collect migration related measurements. These measurements will be
// sent upstream to LaunchDarkly servers and used to enhance the visibility of in progress
// migrations.
type MigrationOpTracker struct {
	flag             *ldmodel.FeatureFlag
	defaultStage     ldmigration.Stage
	op               *ldmigration.Operation
	context          ldcontext.Context
	evaluation       ldreason.EvaluationDetail
	invoked          map[ldmigration.Origin]struct{}
	consistencyCheck *ldmigration.ConsistencyCheck
	errors           map[ldmigration.Origin]struct{}
	latencyMs        map[ldmigration.Origin]int
	sampler          *ldsampling.RatioSampler

	lock sync.Mutex
}

// NewMigrationOpTracker creates a tracker instance that can be used to capture migration related
// measurement data.
//
// By default, the MigrationOpTracker is invalid. You must set an operation using
// [MigrationOpTracker.Operation] before the tracker can generate valid event date using
// [MigrationOpTracker.Build].
func NewMigrationOpTracker(
	flag *ldmodel.FeatureFlag, context ldcontext.Context, detail ldreason.EvaluationDetail, defaultStage ldmigration.Stage,
) *MigrationOpTracker {
	return &MigrationOpTracker{
		flag:         flag,
		defaultStage: defaultStage,
		invoked:      make(map[ldmigration.Origin]struct{}),
		context:      context,
		evaluation:   detail,
		errors:       make(map[ldmigration.Origin]struct{}),
		latencyMs:    make(map[ldmigration.Origin]int),
		sampler:      ldsampling.NewSampler(),
	}
}

// Operation sets the migration related operation associated with these tracking measurements.
func (t *MigrationOpTracker) Operation(op ldmigration.Operation) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.op = &op
}

// TrackInvoked allows recording which origins were called during a migration.
func (t *MigrationOpTracker) TrackInvoked(origin ldmigration.Origin) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.invoked[origin] = struct{}{}
}

// TrackConsistency allows recording the results of a consistency check, along with the
// sampling ratio used to collect that information.
func (t *MigrationOpTracker) TrackConsistency(wasConsistent bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	samplingRatio := 1
	if t.flag != nil && t.flag.Migration != nil {
		samplingRatio = t.flag.Migration.CheckRatio.OrElse(1)
	}

	if !t.sampler.Sample(samplingRatio) {
		return
	}

	t.consistencyCheck = ldmigration.NewConsistencyCheck(wasConsistent, samplingRatio)
}

// TrackError allows recording whether or not an error occurred during the operation.
func (t *MigrationOpTracker) TrackError(origin ldmigration.Origin) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.errors[origin] = struct{}{}
}

// TrackLatency allows tracking the recorded latency for an individual operation.
func (t *MigrationOpTracker) TrackLatency(origin ldmigration.Origin, duration time.Duration) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.latencyMs[origin] = int(duration.Milliseconds())
}

// Build creates an instance of [ldevents.MigrationOpEventData]. This event data can be provided to
// the [LDClient.TrackMigrationOp] method to rely this metric information upstream to LaunchDarkly
// services.
func (t *MigrationOpTracker) Build() (*ldevents.MigrationOpEventData, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.flag == nil {
		return nil, errors.New("migration op tracker was created without an associated flag")
	}

	if len(t.flag.Key) == 0 {
		return nil, errors.New("migration operation cannot contain an empty flag key")
	}

	if len(t.invoked) == 0 {
		return nil, errors.New("no origins were recorded as being invoked")
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
		Op:               *t.op,
		FlagKey:          t.flag.Key,
		Default:          t.defaultStage,
		Evaluation:       t.evaluation,
		Invoked:          t.invoked,
		ConsistencyCheck: t.consistencyCheck,
		Error:            t.errors,
		Latency:          t.latencyMs,
	}, nil
}
