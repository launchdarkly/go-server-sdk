package ldclient

import (
	"math/rand"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldsampling"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/stretchr/testify/assert"
)

var allOrigins = []ldmigration.Origin{ldmigration.Old, ldmigration.New}

func minimalTracker(samplingRatio int) *MigrationOpTracker {
	params := ldbuilders.NewMigrationFlagParametersBuilder().CheckRatio(samplingRatio).Build()
	flag := ldbuilders.NewFlagBuilder("flag-key").MigrationFlagParameters(params).Build()
	context := ldcontext.New("user-key")
	detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
	tracker := NewMigrationOpTracker(&flag, context, detail, ldmigration.Live)
	tracker.Operation(ldmigration.Write)
	tracker.TrackInvoked(ldmigration.Old)
	tracker.TrackInvoked(ldmigration.New)

	return tracker
}

func TestTrackerCanBuildSuccessfully(t *testing.T) {
	tracker := minimalTracker(1)
	event, err := tracker.Build()

	assert.NotNil(t, event)
	assert.NoError(t, err)
}

func TestTrackerCanTrackInvocations(t *testing.T) {
	t.Run("for both origins", func(t *testing.T) {
		tracker := minimalTracker(1)
		// Reset since the minimal tracker already includes some values
		tracker.invoked = make(map[ldmigration.Origin]struct{})

		tracker.TrackInvoked(ldmigration.New)
		tracker.TrackInvoked(ldmigration.Old)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Len(t, event.Invoked, 2)
		if _, ok := event.Invoked[ldmigration.New]; !ok {
			assert.Fail(t, "event is missing new origin invoked")
		}
		if _, ok := event.Invoked[ldmigration.Old]; !ok {
			assert.Fail(t, "event is missing old origin invoked")
		}
	})

	t.Run("for individual origins", func(t *testing.T) {
		for _, origin := range allOrigins {
			tracker := minimalTracker(1)
			// Reset since the minimal tracker already includes some values
			tracker.invoked = make(map[ldmigration.Origin]struct{})

			tracker.TrackInvoked(origin)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			assert.Len(t, event.Invoked, 1)
			if _, ok := event.Invoked[origin]; !ok {
				assert.Failf(t, "event is missing %s origin invoked", string(origin))
			}
		}
	})
}

func TestTrackerCanTrackErrors(t *testing.T) {
	t.Run("for both origins", func(t *testing.T) {
		tracker := minimalTracker(1)
		tracker.TrackError(ldmigration.New)
		tracker.TrackError(ldmigration.Old)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Len(t, event.Error, 2)
		if _, ok := event.Error[ldmigration.New]; !ok {
			assert.Fail(t, "event is missing new origin error")
		}
		if _, ok := event.Error[ldmigration.Old]; !ok {
			assert.Fail(t, "event is missing old origin error")
		}
	})

	t.Run("for individual origins", func(t *testing.T) {
		for _, origin := range allOrigins {
			tracker := minimalTracker(1)
			tracker.TrackError(origin)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			assert.Len(t, event.Error, 1)
			if _, ok := event.Error[origin]; !ok {
				assert.Failf(t, "event is missing %s origin error", string(origin))
			}
		}
	})
}

func TestTrackerCanTrackLatency(t *testing.T) {
	t.Run("for both origins", func(t *testing.T) {
		tracker := minimalTracker(1)
		tracker.TrackLatency(ldmigration.New, 5*time.Second)
		tracker.TrackLatency(ldmigration.Old, 10*time.Second)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Equal(t, event.Latency[ldmigration.New], 5_000)
		assert.Equal(t, event.Latency[ldmigration.Old], 10_000)
		assert.Len(t, event.Latency, 2)
	})

	t.Run("for individual origins", func(t *testing.T) {
		for _, origin := range allOrigins {
			tracker := minimalTracker(1)
			tracker.TrackLatency(origin, 5*time.Second)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			assert.Equal(t, event.Latency[origin], 5_000)
			assert.Len(t, event.Latency, 1)
		}
	})
}

func TestTrackerCanTrackConsistency(t *testing.T) {
	t.Run("defaults to sampling ratio of 1", func(t *testing.T) {
		tracker := minimalTracker(1)
		tracker.TrackConsistency(true)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Equal(t, event.ConsistencyCheck.Consistent(), true)
		assert.Equal(t, event.ConsistencyCheck.SamplingRatio(), 1)
	})

	t.Run("can disable consistency check with 0 sampling ratio", func(t *testing.T) {
		tracker := minimalTracker(0)
		tracker.TrackConsistency(true)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Nil(t, event.ConsistencyCheck)
	})

	t.Run("honors sampling ratio", func(t *testing.T) {
		source := rand.NewSource(1)
		sampler := ldsampling.NewSamplerFromSource(source)

		consistencyWasChecked := 0
		for i := 0; i < 1_000; i++ {
			tracker := minimalTracker(10)
			tracker.sampler = sampler
			tracker.TrackConsistency(true)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			if event.ConsistencyCheck != nil {
				consistencyWasChecked += 1
			}
		}

		// Randomization isn't perfect, but this is pretty close
		assert.Equal(t, 117, consistencyWasChecked)
	})
}

func TestTrackerCannotBuild(t *testing.T) {
	t.Run("without calling invoked", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("flag-key").Build()
		context := ldcontext.New("user-key")
		detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
		tracker := NewMigrationOpTracker(&flag, context, detail, ldmigration.Live)
		tracker.Operation(ldmigration.Write)

		event, err := tracker.Build()

		assert.Nil(t, event)
		assert.Error(t, err)

		assert.Equal(t, err.Error(), "no origins were recorded as being invoked")
	})

	t.Run("without operation", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("flag-key").Build()
		context := ldcontext.New("user-key")
		detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
		tracker := NewMigrationOpTracker(&flag, context, detail, ldmigration.Live)
		tracker.TrackInvoked(ldmigration.Old)

		event, err := tracker.Build()

		assert.Nil(t, event)
		assert.Error(t, err)

		assert.Equal(t, err.Error(), "migration operation not specified")
	})

	t.Run("with invalid context", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("flag-key").Build()
		context := ldcontext.New("")
		detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
		tracker := NewMigrationOpTracker(&flag, context, detail, ldmigration.Live)
		tracker.Operation(ldmigration.Write)
		tracker.TrackInvoked(ldmigration.Old)

		event, err := tracker.Build()

		assert.Nil(t, event)
		assert.Error(t, err)

		assert.Contains(t, err.Error(), "invalid context given")
	})
}
