package hooks

import (
	gocontext "context"
	"errors"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v4/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v4/ldlog"
	"github.com/launchdarkly/go-sdk-common/v4/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
)

func emptyTrackExecutionAssertions(t *testing.T, res *TrackExecution, ldContext ldcontext.Context) {
	assert.Empty(t, res.hooks)
	assert.Equal(t, ldContext, res.context.Context())
	assert.Equal(t, "test-event", res.context.Key())
	assert.Nil(t, res.context.MetricValue())
	assert.Equal(t, ldvalue.Null(), res.context.Data())
}

func TestTrackExecution(t *testing.T) {
	ldContext := ldcontext.New("test-context")

	t.Run("with no hooks", func(t *testing.T) {
		loggers := sharedtest.NewTestLoggers()
		execution := TrackExecution{
			hooks:   []ldhooks.Hook{},
			context: ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}),
			loggers: &loggers,
		}
		execution.AfterTrack(gocontext.Background())
		emptyTrackExecutionAssertions(t, &execution, ldContext)
	})

	t.Run("with hooks", func(t *testing.T) {
		t.Run("verify execution order", func(t *testing.T) {
			loggers := sharedtest.NewTestLoggers()
			tracker := newOrderTracker()
			hookA := createOrderTrackingTrackHook("a", tracker)
			hookB := createOrderTrackingTrackHook("b", tracker)
			execution := TrackExecution{
				hooks:   []ldhooks.Hook{hookA, hookB},
				context: ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}),
				loggers: &loggers,
			}
			execution.AfterTrack(gocontext.Background())
			assert.Equal(t, []string{"a", "b"}, tracker.orderAfter)
		})

		t.Run("run after track", func(t *testing.T) {
			loggers := sharedtest.NewTestLoggers()
			hookA := sharedtest.NewTestHook("a")
			hookB := sharedtest.NewTestHook("b")

			execution := TrackExecution{
				hooks:   []ldhooks.Hook{hookA, hookB},
				context: ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}),
				loggers: &loggers,
			}
			execution.AfterTrack(gocontext.Background())

			hookA.Verify(t, sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageAfterTrack,
				TrackCapture: sharedtest.HookTrackCapture{
					GoContext:          gocontext.Background(),
					TrackSeriesContext: ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}),
				},
			})

			hookB.Verify(t, sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageAfterTrack,
				TrackCapture: sharedtest.HookTrackCapture{
					GoContext:          gocontext.Background(),
					TrackSeriesContext: ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}),
				},
			})
		})

		t.Run("run after track with an error", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()

			hookA := sharedtest.NewTestHook("a")
			// The hooks execute in forward order, so we have an error in A and check that B still executes.
			hookA.AfterTrackInject = func(ctx gocontext.Context, tsc ldhooks.TrackSeriesContext) error {
				return errors.New("something bad")
			}

			hookB := sharedtest.NewTestHook("b")
			hookB.AfterTrackInject = func(ctx gocontext.Context, tsc ldhooks.TrackSeriesContext) error {
				return nil
			}

			execution := TrackExecution{
				hooks:   []ldhooks.Hook{hookA, hookB},
				context: ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}),
				loggers: &mockLog.Loggers,
			}
			execution.AfterTrack(gocontext.Background())

			assert.Len(t, execution.hooks, 2)
			assert.Equal(t, execution.context, ldhooks.NewTrackSeriesContext(ldContext, "test-event", nil, ldvalue.Null(), ldvalue.OptionalString{}))
			assert.Equal(t, []string{"During tracking of event \"test-event\", an error was encountered in \"AfterTrack\" of the \"a\" hook: something bad"},
				mockLog.GetOutput(ldlog.Error))
		})
	})
}

func createOrderTrackingTrackHook(name string, tracker *orderTracker) sharedtest.TestHook {
	h := sharedtest.NewTestHook(name)
	h.AfterTrackInject = func(ctx gocontext.Context, seriesContext ldhooks.TrackSeriesContext) error {
		tracker.orderAfter = append(tracker.orderAfter, name)
		return nil
	}
	return h
}
