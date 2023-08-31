package ldclient

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/stretchr/testify/assert"
)

func makeMigrationFlag(key, stage string) ldmodel.FeatureFlag {
	return ldbuilders.
		NewFlagBuilder(key).
		Variations(ldvalue.String(stage)).
		OffVariation(0).
		Build()
}

func defaultMigrator(client *LDClient) *MigratorBuilder {
	migrator := Migration(client).
		TrackLatency(false).
		TrackErrors(false).
		Read(
			func(interface{}) (interface{}, error) { return false, nil },
			func(interface{}) (interface{}, error) { return false, nil },
			nil,
		).
		Write(
			func(interface{}) (interface{}, error) { return false, nil },
			func(interface{}) (interface{}, error) { return false, nil },
		)

	return migrator
}

func TestMigratorSetsBasicEventValues(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(makeMigrationFlag("key", "off"))

		migrator, err := defaultMigrator(p.client).
			Read(
				func(interface{}) (interface{}, error) { return true, nil },
				func(interface{}) (interface{}, error) { return true, nil },
				nil,
			).
			Write(
				func(interface{}) (interface{}, error) { return true, nil },
				func(interface{}) (interface{}, error) { return true, nil },
			).
			Build()

		assert.NoError(t, err)

		_ = migrator.ValidateRead("key", ldcontext.New("user-key"), ldmigration.Complete, nil)
		assert.Len(t, p.events.Events, 2)

		_ = migrator.ValidateWrite("key", ldcontext.New("user-key"), ldmigration.Complete, nil)
		assert.Len(t, p.events.Events, 4)

		readOpEvent := p.events.Events[1].(ldevents.MigrationOpEventData)  // Ignore evaluation data event
		writeOpEvent := p.events.Events[3].(ldevents.MigrationOpEventData) // Ignore evaluation data event

		assert.Equal(t, ldmigration.Read, readOpEvent.Op)
		assert.Equal(t, "key", readOpEvent.FlagKey)
		assert.EqualValues(t, ldmigration.Complete, readOpEvent.Default)
		assert.EqualValues(t, ldmigration.Off, readOpEvent.Evaluation.Value.StringValue())
		assert.Equal(t, ldvalue.NewOptionalInt(0), readOpEvent.Evaluation.VariationIndex)
		assert.Equal(t, ldreason.NewEvalReasonOff(), readOpEvent.Evaluation.Reason)
		assert.Len(t, readOpEvent.Latency, 0)
		assert.Len(t, readOpEvent.Error, 0)

		assert.EqualValues(t, ldmigration.Write, writeOpEvent.Op)
		assert.Equal(t, "key", writeOpEvent.FlagKey)
		assert.EqualValues(t, ldmigration.Complete, writeOpEvent.Default)
		assert.EqualValues(t, ldmigration.Off, writeOpEvent.Evaluation.Value.StringValue())
		assert.Equal(t, ldvalue.NewOptionalInt(0), writeOpEvent.Evaluation.VariationIndex)
		assert.Equal(t, ldreason.NewEvalReasonOff(), writeOpEvent.Evaluation.Reason)
		assert.Len(t, writeOpEvent.Latency, 0)
		assert.Len(t, writeOpEvent.Error, 0)
	})
}

func TestMigratorTracksLatency(t *testing.T) {
	testParams := []struct {
		Flag              ldmodel.FeatureFlag
		MeasurementCounts int
		OldValue          int
		NewValue          int
	}{
		{
			Flag:              makeMigrationFlag("key", "off"),
			MeasurementCounts: 1,
			OldValue:          500_000,
			NewValue:          0,
		},
		{
			Flag:              makeMigrationFlag("key", "dualwrite"),
			MeasurementCounts: 1,
			OldValue:          500_000,
			NewValue:          0,
		},
		{
			Flag:              makeMigrationFlag("key", "shadow"),
			MeasurementCounts: 2,
			OldValue:          500_000,
			NewValue:          300_000,
		},
		{
			Flag:              makeMigrationFlag("key", "live"),
			MeasurementCounts: 2,
			OldValue:          500_000,
			NewValue:          300_000,
		},
		{
			Flag:              makeMigrationFlag("key", "rampdown"),
			MeasurementCounts: 1,
			OldValue:          0,
			NewValue:          300_000,
		},
		{
			Flag:              makeMigrationFlag("key", "complete"),
			MeasurementCounts: 1,
			OldValue:          0,
			NewValue:          300_000,
		},
	}

	for _, testParam := range testParams {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(testParam.Flag)

			migrator, err := defaultMigrator(p.client).
				TrackLatency(true).
				Read(
					func(interface{}) (interface{}, error) { time.Sleep(500 * time.Millisecond); return true, nil },
					func(interface{}) (interface{}, error) { time.Sleep(300 * time.Millisecond); return true, nil },
					nil,
				).
				Build()

			assert.NoError(t, err)

			result := migrator.ValidateRead("key", ldcontext.New("user-key"), ldmigration.Complete, nil)

			assert.True(t, result.IsSuccess())
			assert.Equal(t, true, result.GetResult())
			assert.Len(t, p.events.Events, 2)

			event := p.events.Events[1].(ldevents.MigrationOpEventData) // Ignore evaluation data event

			assert.Len(t, event.Latency, testParam.MeasurementCounts)
			assert.LessOrEqual(t, event.Latency[ldmigration.Old], testParam.OldValue)
			assert.LessOrEqual(t, event.Latency[ldmigration.New], testParam.NewValue)
		})
	}
}

func TestMigratorTracksErrors(t *testing.T) {
	testParams := []struct {
		Flag        ldmodel.FeatureFlag
		ErrorCounts int
		OldError    bool
		NewError    bool
	}{
		{
			Flag:        makeMigrationFlag("key", "off"),
			ErrorCounts: 1,
			OldError:    true,
			NewError:    false,
		},
		{
			Flag:        makeMigrationFlag("key", "dualwrite"),
			ErrorCounts: 1,
			OldError:    true,
			NewError:    false,
		},
		{
			Flag:        makeMigrationFlag("key", "shadow"),
			ErrorCounts: 2,
			OldError:    true,
			NewError:    true,
		},
		{
			Flag:        makeMigrationFlag("key", "live"),
			ErrorCounts: 2,
			OldError:    true,
			NewError:    true,
		},
		{
			Flag:        makeMigrationFlag("key", "rampdown"),
			ErrorCounts: 1,
			OldError:    false,
			NewError:    true,
		},
		{
			Flag:        makeMigrationFlag("key", "complete"),
			ErrorCounts: 1,
			OldError:    false,
			NewError:    true,
		},
	}

	for _, testParam := range testParams {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(testParam.Flag)

			migrator, err := defaultMigrator(p.client).
				TrackErrors(true).
				Read(
					func(interface{}) (interface{}, error) { return nil, errors.New("error") },
					func(interface{}) (interface{}, error) { return nil, errors.New("error") },
					nil,
				).
				Build()

			assert.NoError(t, err)

			result := migrator.ValidateRead("key", ldcontext.New("user-key"), ldmigration.Complete, nil)

			assert.False(t, result.IsSuccess())
			assert.Nil(t, result.GetResult())
			assert.Error(t, result.GetError(), "error")
			assert.Len(t, p.events.Events, 2)

			event := p.events.Events[1].(ldevents.MigrationOpEventData) // Ignore evaluation data event

			assert.Len(t, event.Error, testParam.ErrorCounts)

			_, ok := event.Error[ldmigration.Old]
			assert.Equal(t, testParam.OldError, ok)

			_, ok = event.Error[ldmigration.New]
			assert.Equal(t, testParam.NewError, ok)
		})
	}
}

func TestMigratorTracksConsistency(t *testing.T) {
	testParams := []struct {
		OldResult      string
		NewResult      string
		ExpectedResult bool
	}{
		{
			OldResult:      "same",
			NewResult:      "same",
			ExpectedResult: true,
		},
		{
			OldResult:      "same",
			NewResult:      "different",
			ExpectedResult: false,
		},
	}

	for _, testParam := range testParams {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(makeMigrationFlag("key", "shadow"))

			var compare MigrationComparisonFn = func(old interface{}, new interface{}) bool {
				return old == new
			}

			migrator, err := defaultMigrator(p.client).
				Read(
					func(interface{}) (interface{}, error) { return testParam.OldResult, nil },
					func(interface{}) (interface{}, error) { return testParam.NewResult, nil },
					&compare,
				).
				Build()

			assert.NoError(t, err)

			result := migrator.ValidateRead("key", ldcontext.New("user-key"), ldmigration.Complete, nil)

			assert.True(t, result.IsSuccess())
			assert.Equal(t, testParam.OldResult, result.GetResult())
			assert.NoError(t, result.GetError())
			assert.Len(t, p.events.Events, 2)

			event := p.events.Events[1].(ldevents.MigrationOpEventData) // Ignore evaluation data event

			assert.Equal(t, testParam.ExpectedResult, event.ConsistencyCheck.Consistent())
			assert.Equal(t, 1, event.ConsistencyCheck.SamplingRatio())
		})
	}
}

func TestMigratorPassingPayloadThroughCorrectly(t *testing.T) {
	t.Run("writes", func(t *testing.T) {
		testParams := []struct {
			Flag ldmodel.FeatureFlag
		}{
			{Flag: makeMigrationFlag("key", "off")},
			{Flag: makeMigrationFlag("key", "dualwrite")},
			{Flag: makeMigrationFlag("key", "shadow")},
			{Flag: makeMigrationFlag("key", "live")},
			{Flag: makeMigrationFlag("key", "rampdown")},
			{Flag: makeMigrationFlag("key", "complete")},
		}

		for _, testParam := range testParams {
			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.data.UsePreconfiguredFlag(testParam.Flag)

				var oldBody, newBody string
				migrator, err := defaultMigrator(p.client).
					Write(
						func(payload interface{}) (interface{}, error) { oldBody = payload.(string); return "old", nil },
						func(payload interface{}) (interface{}, error) { newBody = payload.(string); return "new", nil },
					).
					Build()

				assert.NoError(t, err)

				result := migrator.ValidateWrite("key", ldcontext.New("user-key"), ldmigration.Complete, "payload")
				assert.True(t, result.GetAuthoritativeResult().IsSuccess())
				if result.GetAuthoritativeResult().GetOrigin() == ldmigration.Old {
					assert.Equal(t, oldBody, "payload")
				} else {
					assert.Equal(t, newBody, "payload")
				}

				if result.GetNonAuthoritativeResult() != nil {
					if result.GetNonAuthoritativeResult().GetOrigin() == ldmigration.Old {
						assert.Equal(t, oldBody, "payload")
					} else {
						assert.Equal(t, newBody, "payload")
					}
				}
			})
		}
	})

	t.Run("reads", func(t *testing.T) {
		testParams := []struct {
			Flag        ldmodel.FeatureFlag
			ExpectedOld bool
			ExpectedNew bool
		}{
			{Flag: makeMigrationFlag("key", "off"), ExpectedOld: true},
			{Flag: makeMigrationFlag("key", "dualwrite"), ExpectedOld: true},
			{Flag: makeMigrationFlag("key", "shadow"), ExpectedOld: true, ExpectedNew: true},
			{Flag: makeMigrationFlag("key", "live"), ExpectedOld: true, ExpectedNew: true},
			{Flag: makeMigrationFlag("key", "rampdown"), ExpectedNew: true},
			{Flag: makeMigrationFlag("key", "complete"), ExpectedNew: true},
		}

		for _, testParam := range testParams {
			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.data.UsePreconfiguredFlag(testParam.Flag)

				var oldBody, newBody string
				migrator, err := defaultMigrator(p.client).
					Read(
						func(payload interface{}) (interface{}, error) { oldBody = payload.(string); return "old", nil },
						func(payload interface{}) (interface{}, error) { newBody = payload.(string); return "new", nil },
						nil,
					).
					Build()

				assert.NoError(t, err)

				migrator.ValidateRead("key", ldcontext.New("user-key"), ldmigration.Complete, "payload")
				if testParam.ExpectedOld {
					assert.Equal(t, oldBody, "payload")
				} else {
					assert.Empty(t, oldBody, "payload")
				}

				if testParam.ExpectedNew {
					assert.Equal(t, newBody, "payload")
				} else {
					assert.Empty(t, newBody, "payload")
				}
			})
		}
	})
}

func TestMigratorWriteReturnsCorrectAuthoritativeResults(t *testing.T) {
	testParams := []struct {
		Flag                   ldmodel.FeatureFlag
		AuthoritativeResult    string
		NonAuthoritativeResult ldvalue.OptionalString
	}{
		{
			Flag:                makeMigrationFlag("key", "off"),
			AuthoritativeResult: "old",
		},
		{
			Flag:                   makeMigrationFlag("key", "dualwrite"),
			AuthoritativeResult:    "old",
			NonAuthoritativeResult: ldvalue.NewOptionalString("new"),
		},
		{
			Flag:                   makeMigrationFlag("key", "shadow"),
			AuthoritativeResult:    "old",
			NonAuthoritativeResult: ldvalue.NewOptionalString("new"),
		},
		{
			Flag:                   makeMigrationFlag("key", "live"),
			AuthoritativeResult:    "new",
			NonAuthoritativeResult: ldvalue.NewOptionalString("old"),
		},
		{
			Flag:                   makeMigrationFlag("key", "rampdown"),
			AuthoritativeResult:    "new",
			NonAuthoritativeResult: ldvalue.NewOptionalString("old"),
		},
		{
			Flag:                makeMigrationFlag("key", "complete"),
			AuthoritativeResult: "new",
		},
	}

	for _, testParam := range testParams {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(testParam.Flag)

			migrator, err := defaultMigrator(p.client).
				Write(
					func(interface{}) (interface{}, error) { return "old", nil },
					func(interface{}) (interface{}, error) { return "new", nil },
				).
				Build()

			assert.NoError(t, err)

			result := migrator.ValidateWrite("key", ldcontext.New("user-key"), ldmigration.Complete, nil)

			assert.True(t, result.GetAuthoritativeResult().IsSuccess())
			assert.Equal(t, testParam.AuthoritativeResult, result.GetAuthoritativeResult().GetResult())

			if testParam.NonAuthoritativeResult.IsDefined() {
				assert.True(t, result.GetNonAuthoritativeResult().IsSuccess())
				assert.Equal(t, testParam.NonAuthoritativeResult.String(), result.GetNonAuthoritativeResult().GetResult())
			} else {
				assert.Nil(t, result.GetNonAuthoritativeResult())
			}
		})
	}
}

func TestMigratorWriteStopsOnAuthoritativeFailure(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(makeMigrationFlag("key", "dualwrite"))

		migrator, err := defaultMigrator(p.client).
			Write(
				func(interface{}) (interface{}, error) { return nil, errors.New("old is failing") },
				func(interface{}) (interface{}, error) { return "new", nil },
			).
			Build()

		assert.NoError(t, err)

		result := migrator.ValidateWrite("key", ldcontext.New("user-key"), ldmigration.Complete, nil)

		assert.False(t, result.GetAuthoritativeResult().IsSuccess())
		assert.Error(t, result.GetAuthoritativeResult().GetError(), "old is failing")
		assert.Nil(t, result.GetNonAuthoritativeResult())
	})
}

func TestMigratorWriteReturnsResultOnNonAuthoritativeFailure(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(makeMigrationFlag("key", "dualwrite"))

		migrator, err := defaultMigrator(p.client).
			Write(
				func(interface{}) (interface{}, error) { return "old", nil },
				func(interface{}) (interface{}, error) { return nil, errors.New("new is failing") },
			).
			Build()

		assert.NoError(t, err)

		result := migrator.ValidateWrite("key", ldcontext.New("user-key"), ldmigration.Complete, nil)

		assert.True(t, result.GetAuthoritativeResult().IsSuccess())
		assert.Equal(t, "old", result.GetAuthoritativeResult().GetResult())

		assert.False(t, result.GetNonAuthoritativeResult().IsSuccess())
		assert.Error(t, result.GetNonAuthoritativeResult().GetError(), "old is failing")
	})
}
