package ldclient

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/stretchr/testify/assert"
)

var migrationTestUser = lduser.NewUser("userkey")

func TestDefaultIsReturnedIfFlagEvaluatesToNonStringType(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("migration-key").Build() // flag is off and we haven't defined an off variation

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		stage, _, err := p.client.MigrationVariation("migration-key", migrationTestUser, ldmigration.Live)

		assert.NoError(t, err)
		assert.Equal(t, ldmigration.Live, stage)
	})
}

func TestDefaultIsReturnedIfMigrationFlagDoesNotExist(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		stage, _, err := p.client.MigrationVariation("migration-key", migrationTestUser, ldmigration.Live)

		assert.NoError(t, err)
		assert.Equal(t, ldmigration.Live, stage)
	})
}

func TestDefaultIsReturnedFlagEvaluatesToInvalidStageValue(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("migration-key").Variations(ldvalue.String("invalid-stage")).OffVariation(0).On(false).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		stage, _, err := p.client.MigrationVariation("migration-key", migrationTestUser, ldmigration.Live)

		assert.Error(t, err)
		assert.Equal(t, ldmigration.Live, stage)
	})
}

func TestCorrectStageCanBeDeterminedFromFlag(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("migration-key").Variations(ldvalue.String("off"), ldvalue.String("dualwrite")).OffVariation(0).On(true).FallthroughVariation(1).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		stage, _, err := p.client.MigrationVariation("migration-key", migrationTestUser, ldmigration.Live)

		assert.NoError(t, err)
		assert.Equal(t, ldmigration.DualWrite, stage)
	})
}
