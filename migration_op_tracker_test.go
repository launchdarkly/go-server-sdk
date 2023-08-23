package ldclient

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/stretchr/testify/assert"
)

var allOrigins = []ldmigration.Origin{ldmigration.Old, ldmigration.New}

func minimalTracker() *MigrationOpTracker {
	context := ldcontext.New("user-key")
	detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
	tracker := NewMigrationOpTracker("flag-key", context, detail, ldmigration.Live)
	tracker.Operation(ldmigration.Write)

	return tracker
}

func TestTrackerCanBuildSuccessfully(t *testing.T) {
	tracker := minimalTracker()
	event, err := tracker.Build()

	assert.NotNil(t, event)
	assert.NoError(t, err)
}

func TestTrackerCanTrackErrors(t *testing.T) {
	t.Run("for both origins", func(t *testing.T) {
		tracker := minimalTracker()
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
			tracker := minimalTracker()
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
		tracker := minimalTracker()
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
			tracker := minimalTracker()
			tracker.TrackLatency(origin, 5*time.Second)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			assert.Equal(t, event.Latency[origin], 5_000)
			assert.Len(t, event.Latency, 1)
		}
	})
}

func TestTrackerCannotBuild(t *testing.T) {
	t.Run("without operation", func(t *testing.T) {
		context := ldcontext.New("user-key")
		detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
		tracker := NewMigrationOpTracker("flag-key", context, detail, ldmigration.Live)

		event, err := tracker.Build()

		assert.Nil(t, event)
		assert.Error(t, err)
	})

	t.Run("with invalid context", func(t *testing.T) {
		context := ldcontext.New("")
		detail := ldreason.NewEvaluationDetail(ldvalue.Bool(true), 1, ldreason.NewEvalReasonFallthrough())
		tracker := NewMigrationOpTracker("flag-key", context, detail, ldmigration.Live)
		tracker.Operation(ldmigration.Write)

		event, err := tracker.Build()

		assert.Nil(t, event)
		assert.Error(t, err)
	})
}
