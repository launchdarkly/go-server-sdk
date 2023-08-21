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
		tracker.TrackError(ldmigration.New, true)
		tracker.TrackError(ldmigration.Old, false)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.True(t, event.Error[ldmigration.New])
		assert.False(t, event.Error[ldmigration.Old])
		assert.Len(t, event.Error, 2)
	})

	t.Run("for individual origins", func(t *testing.T) {
		for _, origin := range allOrigins {
			tracker := minimalTracker()
			tracker.TrackError(origin, true)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			assert.True(t, event.Error[origin])
			assert.Len(t, event.Error, 1)
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

func TestTrackerCanTrackCustomMeasurements(t *testing.T) {
	t.Run("for both origins", func(t *testing.T) {
		tracker := minimalTracker()
		tracker.TrackCustom("custom-name", ldmigration.New, 30)
		tracker.TrackCustom("custom-name", ldmigration.Old, 60)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Equal(t, event.CustomMeasurements["custom-name"][ldmigration.New], float64(30))
		assert.Equal(t, event.CustomMeasurements["custom-name"][ldmigration.Old], float64(60))
		assert.Len(t, event.CustomMeasurements, 1)
		assert.Len(t, event.CustomMeasurements["custom-name"], 2)
	})

	t.Run("for individual origins", func(t *testing.T) {
		for _, origin := range allOrigins {
			tracker := minimalTracker()
			tracker.TrackCustom("custom-name", origin, 30)

			event, err := tracker.Build()

			assert.NoError(t, err)
			assert.NotNil(t, event)

			assert.Equal(t, event.CustomMeasurements["custom-name"][origin], float64(30))
			assert.Len(t, event.CustomMeasurements, 1)
			assert.Len(t, event.CustomMeasurements["custom-name"], 1)
		}
	})

	t.Run("for multiple metrics", func(t *testing.T) {
		tracker := minimalTracker()
		tracker.TrackCustom("one-custom", ldmigration.New, 30)
		tracker.TrackCustom("two-custom", ldmigration.Old, 60)

		event, err := tracker.Build()

		assert.NoError(t, err)
		assert.NotNil(t, event)

		assert.Equal(t, event.CustomMeasurements["one-custom"][ldmigration.New], float64(30))
		assert.Equal(t, event.CustomMeasurements["two-custom"][ldmigration.Old], float64(60))
		assert.Len(t, event.CustomMeasurements, 2)
		assert.Len(t, event.CustomMeasurements["one-custom"], 1)
		assert.Len(t, event.CustomMeasurements["two-custom"], 1)
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
