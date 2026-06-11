package ldclient

import (
	gocontext "context"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/stretchr/testify/assert"
)

var migrationTestUser = lduser.NewUser("userkey")

type MigrationVariationMethod = func(
	client *LDClient,
	key string,
	context ldcontext.Context,
	stage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error)

func TestMigrationVariation(t *testing.T) {

	t.Run("with MigrationVariation", func(t *testing.T) {
		runMigrationTests(t, func(client *LDClient,
			key string,
			context ldcontext.Context,
			stage ldmigration.Stage,
		) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
			return client.MigrationVariation(key, context, stage)
		})
	})

	t.Run("with MigrationVariationCtx", func(t *testing.T) {
		runMigrationTests(t, func(client *LDClient,
			key string,
			context ldcontext.Context,
			stage ldmigration.Stage,
		) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
			return client.MigrationVariationCtx(gocontext.TODO(), key, context, stage)
		})
	})

}

func runMigrationTests(t *testing.T, method MigrationVariationMethod) {
	t.Run("default is returned if flag evaluates to non string type", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("migration-key").Build() // flag is off and we haven't defined an off variation

		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(flag)

			stage, _, err := method(p.client, "migration-key", migrationTestUser, ldmigration.Live)

			assert.NoError(t, err)
			assert.EqualValues(t, ldmigration.Live, stage)
		})
	})

	t.Run("default is returned if migration flag does not exist", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			stage, _, err := method(p.client, "migration-key", migrationTestUser, ldmigration.Live)

			assert.NoError(t, err)
			assert.EqualValues(t, ldmigration.Live, stage)
		})
	})

	t.Run("default is returned if flag evaluates to an invalid stage", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("migration-key").Variations(ldvalue.String("invalid-stage")).OffVariation(0).On(false).Build()

		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(flag)

			stage, _, err := method(p.client, "migration-key", migrationTestUser, ldmigration.Live)

			assert.Error(t, err)
			assert.EqualValues(t, ldmigration.Live, stage)
		})
	})

	t.Run("correct stage can be returned from flag", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("migration-key").Variations(ldvalue.String("off"), ldvalue.String("dualwrite")).OffVariation(0).On(true).FallthroughVariation(1).Build()

		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(flag)

			stage, _, err := method(p.client, "migration-key", migrationTestUser, ldmigration.Live)

			assert.NoError(t, err)
			assert.EqualValues(t, ldmigration.DualWrite, stage)
		})
	})
}
