package ldclient

import (
	gocontext "context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
)

func TestScopedClientCurrentContext(t *testing.T) {
	ldctx := ldcontext.New("user1")
	c := makeTestClient().ForContext(ldctx)

	assert.Equal(t, ldctx, c.CurrentContext())
}

func TestScopedClientCollectsContexts(t *testing.T) {
	ldctx1 := ldcontext.NewWithKind("foo", "foo1")
	ldctx2 := ldcontext.NewMulti(
		ldcontext.NewWithKind("bar", "bar1"),
		ldcontext.NewWithKind("baz", "baz1"),
	)
	ldctx3 := ldcontext.NewWithKind("qux", "qux1")
	ldctx4 := ldcontext.NewMulti(
		ldcontext.NewWithKind("quux", "quux1"),
		ldcontext.NewWithKind("quuz", "quuz1"),
	)
	dupeCtx := ldcontext.NewWithKind("foo", "foo2")

	t.Run("adding contexts happy path", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		c := makeTestClientWithConfig(func(config *Config) {
			config.Logging = ldcomponents.Logging().Loggers(logCapture.Loggers)
		}).ForContext(ldctx1, ldctx2)

		c.AddContext(ldctx3, ldctx4)

		assert.Equal(t, ldcontext.NewMulti(ldctx1, ldctx2, ldctx3, ldctx4), c.CurrentContext())
		assert.Empty(t, logCapture.GetOutput(ldlog.Warn))
	})

	t.Run("adding duplicate context kinds", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		c := makeTestClientWithConfig(func(config *Config) {
			config.Logging = ldcomponents.Logging().Loggers(logCapture.Loggers)
		}).ForContext(ldctx1)

		c.AddContext(dupeCtx)

		assert.Equal(t, ldcontext.NewMulti(ldctx1), c.CurrentContext())
		logCapture.AssertMessageMatch(t, true, ldlog.Warn, "Tried to add a duplicate foo context to LDScopedClient")
	})

	t.Run("overwriting contexts", func(t *testing.T) {
		c := makeTestClient().ForContext(ldctx1, ldctx2, ldctx3, ldctx4)

		c.OverwriteContextByKind(dupeCtx)
		c.OverwriteContextByKind(ldctx2, ldctx3)

		assert.Equal(t, ldcontext.NewMulti(ldctx2, ldctx3, ldctx4, dupeCtx), c.CurrentContext())
	})
}

// Testing the scoped versions of all the evaluation methods
// Almost the same as the tests in ldclient_evaluation_test.go, but with the scoped client instead

func TestScopedBoolVariation(t *testing.T) {
	expected, defaultVal := true, false

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, err := p.client.ForContext(evalTestUser).BoolVariation(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), noReason)
		})
	})
	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, err := p.client.ForContext(evalTestUser).BoolVariationCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), noReason)
		})
	})
	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, detail, err := p.client.ForContext(evalTestUser).BoolVariationDetail(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Bool(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), detail.Reason)
		})
	})
	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, detail, err := p.client.ForContext(evalTestUser).BoolVariationDetailCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Bool(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), detail.Reason)
		})
	})
}

func TestScopedIntVariation(t *testing.T) {
	expected, defaultVal := 100, 10000

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

			actual, err := p.client.ForContext(evalTestUser).IntVariation(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), noReason)
		})
	})
	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

			actual, err := p.client.ForContext(evalTestUser).IntVariationCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), noReason)
		})
	})
	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

			actual, detail, err := p.client.ForContext(evalTestUser).IntVariationDetail(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Int(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), detail.Reason)
		})
	})
	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

			actual, detail, err := p.client.ForContext(evalTestUser).IntVariationDetailCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Int(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), detail.Reason)
		})
	})
}

func TestScopedFloat64Variation(t *testing.T) {
	expected, defaultVal := 100.01, 0.0

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, err := p.client.ForContext(evalTestUser).Float64Variation(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), noReason)
		})
	})
	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, err := p.client.ForContext(evalTestUser).Float64VariationCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), noReason)
		})
	})
	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, detail, err := p.client.ForContext(evalTestUser).Float64VariationDetail(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Float64(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), detail.Reason)
		})
	})
	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, detail, err := p.client.ForContext(evalTestUser).Float64VariationDetailCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Float64(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), detail.Reason)
		})
	})
}

func TestScopedStringVariation(t *testing.T) {
	expected, defaultVal := "b", "a"

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, err := p.client.ForContext(evalTestUser).StringVariation(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), noReason)
		})
	})
	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, err := p.client.ForContext(evalTestUser).StringVariationCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), noReason)
		})
	})
	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, detail, err := p.client.ForContext(evalTestUser).StringVariationDetail(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.String(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), detail.Reason)
		})
	})
	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, detail, err := p.client.ForContext(evalTestUser).StringVariationDetailCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.String(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), detail.Reason)
		})
	})
}

func TestScopedJSONVariation(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	defaultVal := ldvalue.String("no")

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, err := p.client.ForContext(evalTestUser).JSONVariation(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, noReason)
		})
	})
	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, err := p.client.ForContext(evalTestUser).JSONVariationCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, noReason)
		})
	})
	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, detail, err := p.client.ForContext(evalTestUser).JSONVariationDetail(evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(expected, expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, detail.Reason)
		})
	})
	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, detail, err := p.client.ForContext(evalTestUser).JSONVariationDetailCtx(gocontext.TODO(), evalFlagKey, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(expected, expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, detail.Reason)
		})
	})
}

// Scoped version of ldclient_migration_test.go

func TestScopedMigrationVariation(t *testing.T) {
	t.Run("with MigrationVariation", func(t *testing.T) {
		runMigrationTests(t, func(client *LDClient,
			key string,
			context ldcontext.Context,
			stage ldmigration.Stage,
		) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
			return client.ForContext(context).MigrationVariation(key, stage)
		})
	})

	t.Run("with MigrationVariationCtx", func(t *testing.T) {
		runMigrationTests(t, func(client *LDClient,
			key string,
			context ldcontext.Context,
			stage ldmigration.Stage,
		) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
			return client.ForContext(context).MigrationVariationCtx(gocontext.TODO(), key, stage)
		})
	})

}

// Scoped event tests

func TestScopedTrackCalls(t *testing.T) {
	t.Run("TrackEvent", func(t *testing.T) {
		client := makeTestClient()
		defer client.Close()

		key := "eventKey"
		err := client.ForContext(evalTestUser).TrackEvent(key)
		assert.NoError(t, err)

		events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
		assert.Equal(t, 1, len(events))
		e := events[0].(ldevents.CustomEventData)
		assert.Equal(t, ldevents.Context(evalTestUser), e.Context)
		assert.Equal(t, key, e.Key)
	})
	t.Run("TrackEventCtx", func(t *testing.T) {
		client := makeTestClient()
		defer client.Close()

		key := "eventKey"
		err := client.ForContext(evalTestUser).TrackEventCtx(gocontext.TODO(), key)
		assert.NoError(t, err)

		events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
		assert.Equal(t, 1, len(events))
		e := events[0].(ldevents.CustomEventData)
		assert.Equal(t, ldevents.Context(evalTestUser), e.Context)
		assert.Equal(t, key, e.Key)
	})
	t.Run("TrackData", func(t *testing.T) {
		client := makeTestClient()
		defer client.Close()

		key := "eventKey"
		data := ldvalue.String("data")
		err := client.ForContext(evalTestUser).TrackData(key, data)
		assert.NoError(t, err)

		events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
		assert.Equal(t, 1, len(events))
		e := events[0].(ldevents.CustomEventData)
		assert.Equal(t, ldevents.Context(evalTestUser), e.Context)
		assert.Equal(t, key, e.Key)
		assert.Equal(t, data, e.Data)
	})
	t.Run("TrackDataCtx", func(t *testing.T) {
		client := makeTestClient()
		defer client.Close()

		key := "eventKey"
		data := ldvalue.String("data")
		err := client.ForContext(evalTestUser).TrackDataCtx(gocontext.TODO(), key, data)
		assert.NoError(t, err)

		events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
		assert.Equal(t, 1, len(events))
		e := events[0].(ldevents.CustomEventData)
		assert.Equal(t, ldevents.Context(evalTestUser), e.Context)
		assert.Equal(t, key, e.Key)
		assert.Equal(t, data, e.Data)
	})
	t.Run("TrackMetric", func(t *testing.T) {
		client := makeTestClient()
		defer client.Close()

		key := "eventKey"
		data := ldvalue.String("data")
		metric := float64(1.5)
		err := client.ForContext(evalTestUser).TrackMetric(key, metric, data)
		assert.NoError(t, err)

		events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
		assert.Equal(t, 1, len(events))
		e := events[0].(ldevents.CustomEventData)
		assert.Equal(t, ldevents.Context(evalTestUser), e.Context)
		assert.Equal(t, key, e.Key)
		assert.Equal(t, data, e.Data)
		assert.Equal(t, metric, e.MetricValue)
	})
	t.Run("TrackMetricCtx", func(t *testing.T) {
		client := makeTestClient()
		defer client.Close()

		key := "eventKey"
		data := ldvalue.String("data")
		metric := float64(1.5)
		err := client.ForContext(evalTestUser).TrackMetricCtx(gocontext.TODO(), key, metric, data)
		assert.NoError(t, err)

		events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
		assert.Equal(t, 1, len(events))
		e := events[0].(ldevents.CustomEventData)
		assert.Equal(t, ldevents.Context(evalTestUser), e.Context)
		assert.Equal(t, key, e.Key)
		assert.Equal(t, data, e.Data)
		assert.Equal(t, metric, e.MetricValue)
	})
}
