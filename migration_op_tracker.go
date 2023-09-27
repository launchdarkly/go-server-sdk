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
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// MigrationOpTracker is used to collect migration related measurements. These measurements will be
// sent upstream to LaunchDarkly servers and used to enhance the visibility of in progress
// migrations.
type MigrationOpTracker struct {
	key              string
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
	key string, flag *ldmodel.FeatureFlag, context ldcontext.Context,
	detail ldreason.EvaluationDetail, defaultStage ldmigration.Stage,
) *MigrationOpTracker {
	return &MigrationOpTracker{
		key:          key,
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

// TrackConsistency allows recording the results of a consistency check.
//
// The provided consistency function will be run if the flag's check ratio
// value allows it. Otherwise, the function is skipped and a consistency
// measurement is not included.
func (t *MigrationOpTracker) TrackConsistency(isConsistent func() bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	samplingRatio := 1
	if t.flag != nil && t.flag.Migration != nil {
		samplingRatio = t.flag.Migration.CheckRatio.OrElse(1)
	}

	if !t.sampler.Sample(samplingRatio) {
		return
	}

	t.consistencyCheck = ldmigration.NewConsistencyCheck(isConsistent(), samplingRatio)
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

	if len(t.key) == 0 {
		return nil, errors.New("migration operation cannot contain an empty key")
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

	if err := t.checkConsistency(); err != nil {
		return nil, err
	}

	event := ldevents.MigrationOpEventData{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: ldtime.UnixMillisNow(),
			Context:      ldevents.Context(t.context),
		},
		Op:               *t.op,
		FlagKey:          t.key,
		Default:          t.defaultStage,
		Evaluation:       t.evaluation,
		Invoked:          t.invoked,
		ConsistencyCheck: t.consistencyCheck,
		Error:            t.errors,
		Latency:          t.latencyMs,
	}

	if t.flag != nil {
		event.SamplingRatio = t.flag.SamplingRatio
		event.Version = ldvalue.NewOptionalInt(t.flag.Version)
	}

	return &event, nil
}

func (t *MigrationOpTracker) checkConsistency() error {
	validOrigins := []ldmigration.Origin{ldmigration.Old, ldmigration.New}

	for _, origin := range validOrigins {
		if _, ok := t.invoked[origin]; ok {
			continue
		}

		if _, ok := t.latencyMs[origin]; ok {
			return fmt.Errorf("provided latency for '%s' without recording invocation", origin)
		}

		if _, ok := t.errors[origin]; ok {
			return fmt.Errorf("provided error for '%s' without recording invocation", origin)
		}
	}

	if t.consistencyCheck != nil && len(t.invoked) != 2 {
		return errors.New("provided consistency without recording both invocations")
	}

	return nil
}
