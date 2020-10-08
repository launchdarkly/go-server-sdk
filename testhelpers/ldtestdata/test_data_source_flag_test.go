package ldtestdata

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"
)

const (
	trueVar  = 0
	falseVar = 1
)

func verifyFlag(t *testing.T, configureFlag func(*FlagBuilder), expectedFlag *ldbuilders.FlagBuilder) {
	expectedJSON, _ := json.Marshal(expectedFlag.Build())
	testDataSourceTest(func(p testDataSourceTestParams) {
		p.withDataSource(t, func(interfaces.DataSource) {
			f := p.td.Flag("flagkey")
			configureFlag(f)
			p.td.Update(f)
			up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flagkey", 1, time.Millisecond)
			upJSON := ldstoreimpl.Features().Serialize(up.Item)
			assert.JSONEq(t, string(expectedJSON), string(upJSON))
		})
	})
}

func basicBool() *ldbuilders.FlagBuilder {
	return ldbuilders.NewFlagBuilder("flagkey").Version(1).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		OffVariation(1)
}

func TestFlagConfig(t *testing.T) {
	basicString := func() *ldbuilders.FlagBuilder {
		return ldbuilders.NewFlagBuilder("flagkey").Version(1).On(true).Variations(threeStringValues...)
	}

	t.Run("simple boolean flag", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {}, basicBool().On(true).FallthroughVariation(trueVar))
		verifyFlag(t, func(f *FlagBuilder) { f.BooleanFlag() }, basicBool().On(true).FallthroughVariation(trueVar))
		verifyFlag(t, func(f *FlagBuilder) { f.On(true) }, basicBool().On(true).FallthroughVariation(trueVar))
		verifyFlag(t, func(f *FlagBuilder) { f.On(false) }, basicBool().On(false).FallthroughVariation(trueVar))
		verifyFlag(t, func(f *FlagBuilder) { f.VariationForAllUsers(false) }, basicBool().On(true).FallthroughVariation(falseVar))
		verifyFlag(t, func(f *FlagBuilder) { f.VariationForAllUsers(true) }, basicBool().On(true).FallthroughVariation(trueVar))

		verifyFlag(t, func(f *FlagBuilder) {
			f.FallthroughVariation(true).OffVariation(false)
		}, basicBool().On(true).FallthroughVariation(trueVar).OffVariation(falseVar))

		verifyFlag(t, func(f *FlagBuilder) {
			f.FallthroughVariation(false).OffVariation(true)
		}, basicBool().On(true).FallthroughVariation(falseVar).OffVariation(trueVar))
	})

	t.Run("using boolean config methods forces flag to be boolean", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(ldvalue.Int(1), ldvalue.Int(2))
			f.BooleanFlag()
		}, basicBool().On(true).FallthroughVariation(trueVar))

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(ldvalue.Bool(true), ldvalue.Int(2))
			f.BooleanFlag()
		}, basicBool().On(true).FallthroughVariation(trueVar))

		verifyFlag(t, func(f *FlagBuilder) {
			f.ValueForAllUsers(ldvalue.String("x"))
			f.BooleanFlag()
		}, basicBool().On(true).FallthroughVariation(trueVar))
	})

	t.Run("flag with string variations", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2)
		}, basicString().OffVariation(0).FallthroughVariation(2))
	})

	t.Run("user targets", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", true).VariationForUser("b", true)
		}, basicBool().On(true).FallthroughVariation(trueVar).AddTarget(0, "a", "b"))

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", true).VariationForUser("a", true)
		}, basicBool().On(true).FallthroughVariation(trueVar).AddTarget(0, "a"))

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", false).VariationForUser("b", true).VariationForUser("c", false)
		}, basicBool().On(true).FallthroughVariation(trueVar).AddTarget(0, "b").AddTarget(1, "a", "c"))

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", true).VariationForUser("b", true).VariationForUser("a", false)
		}, basicBool().On(true).FallthroughVariation(trueVar).AddTarget(0, "b").AddTarget(1, "a"))

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
				VariationIndexForUser("a", 2).VariationIndexForUser("b", 2)
		}, basicString().On(true).OffVariation(0).FallthroughVariation(2).AddTarget(2, "a", "b"))

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
				VariationIndexForUser("a", 2).VariationIndexForUser("b", 1).VariationIndexForUser("c", 2)
		}, basicString().On(true).OffVariation(0).FallthroughVariation(2).AddTarget(1, "b").AddTarget(2, "a", "c"))
	})
}

func TestRuleConfig(t *testing.T) {
	t.Run("simple match returning variation 0/true", func(t *testing.T) {
		matchReturnsVariation0 := basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.Clause(lduser.NameAttribute, ldmodel.OperatorIn, ldvalue.String("Lucy")),
			),
		)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(true)
		}, matchReturnsVariation0)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturnIndex(0)
		}, matchReturnsVariation0)
	})

	t.Run("simple match returning variation 1/false", func(t *testing.T) {
		matchReturnsVariation1 := basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(falseVar).Clauses(
				ldbuilders.Clause(lduser.NameAttribute, ldmodel.OperatorIn, ldvalue.String("Lucy")),
			),
		)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(false)
		}, matchReturnsVariation1)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturnIndex(1)
		}, matchReturnsVariation1)
	})

	t.Run("negated match", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfNotMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.Negate(ldbuilders.Clause(lduser.NameAttribute, ldmodel.OperatorIn, ldvalue.String("Lucy"))),
			),
		))
	})

	t.Run("multiple clauses", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).
				AndMatch(lduser.CountryAttribute, ldvalue.String("gb")).
				ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.Clause(lduser.NameAttribute, ldmodel.OperatorIn, ldvalue.String("Lucy")),
				ldbuilders.Clause(lduser.CountryAttribute, ldmodel.OperatorIn, ldvalue.String("gb")),
			),
		))
	})

	t.Run("multiple rules", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(true).
				IfMatch(lduser.NameAttribute, ldvalue.String("Mina")).ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.Clause(lduser.NameAttribute, ldmodel.OperatorIn, ldvalue.String("Lucy")),
			),
		).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule1").Variation(trueVar).Clauses(
				ldbuilders.Clause(lduser.NameAttribute, ldmodel.OperatorIn, ldvalue.String("Mina")),
			),
		))
	})
}
