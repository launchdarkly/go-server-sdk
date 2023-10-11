package ldtestdata

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"

	m "github.com/launchdarkly/go-test-helpers/v3/matchers"
)

const (
	trueVar  = 0
	falseVar = 1
)

func verifyFlag(t *testing.T, configureFlag func(*FlagBuilder), expectedFlag *ldbuilders.FlagBuilder) {
	t.Helper()
	expectedJSON, _ := json.Marshal(expectedFlag.Build())
	testDataSourceTest(t, func(p testDataSourceTestParams) {
		t.Helper()
		p.withDataSource(t, func(subsystems.DataSource) {
			t.Helper()
			f := p.td.Flag("flagkey")
			configureFlag(f)
			p.td.Update(f)
			up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flagkey", 1, time.Millisecond)
			upJSON := ldstoreimpl.Features().Serialize(up.Item)
			m.In(t).Assert(string(upJSON), m.JSONStrEqual(string(expectedJSON)))
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
		verifyFlag(t, func(f *FlagBuilder) { f.VariationForAll(false) }, basicBool().On(true).FallthroughVariation(falseVar))
		verifyFlag(t, func(f *FlagBuilder) { f.VariationForAll(true) }, basicBool().On(true).FallthroughVariation(trueVar))

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
			f.ValueForAll(ldvalue.String("x"))
			f.BooleanFlag()
		}, basicBool().On(true).FallthroughVariation(trueVar))
	})

	t.Run("flag with string variations", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2)
		}, basicString().OffVariation(0).FallthroughVariation(2))

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).VariationForAllIndex(1)
		}, basicString().OffVariation(1).FallthroughVariation(1))
	})
}

func TestFlagTargets(t *testing.T) {
	basicString := func() *ldbuilders.FlagBuilder {
		return ldbuilders.NewFlagBuilder("flagkey").Version(1).On(true).Variations(threeStringValues...)
	}

	t.Run("user targets", func(t *testing.T) {
		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForUser("a", true).VariationForUser("b", true)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddTarget(0, "a", "b").AddContextTarget(ldcontext.DefaultKind, 0))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForUser("a", true).VariationForUser("a", true)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddTarget(0, "a").AddContextTarget(ldcontext.DefaultKind, 0))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForUser("a", false).VariationForUser("b", true).VariationForUser("c", false)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddTarget(0, "b").AddContextTarget(ldcontext.DefaultKind, 0).
				AddTarget(1, "a", "c").AddContextTarget(ldcontext.DefaultKind, 1))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForUser("a", true).VariationForUser("b", true).VariationForUser("a", false)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddTarget(0, "b").AddContextTarget(ldcontext.DefaultKind, 0).
				AddTarget(1, "a").AddContextTarget(ldcontext.DefaultKind, 1))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
					VariationIndexForUser("a", 2).VariationIndexForUser("b", 2)
			},
			basicString().On(true).OffVariation(0).FallthroughVariation(2).
				AddTarget(2, "a", "b").AddContextTarget(ldcontext.DefaultKind, 2))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
					VariationIndexForUser("a", 2).VariationIndexForUser("b", 1).VariationIndexForUser("c", 2)
			},
			basicString().On(true).OffVariation(0).FallthroughVariation(2).
				AddTarget(1, "b").AddContextTarget(ldcontext.DefaultKind, 1).
				AddTarget(2, "a", "c").AddContextTarget(ldcontext.DefaultKind, 2))
	})

	t.Run("context targets", func(t *testing.T) {
		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForKey("org", "a", true).VariationForKey("org", "b", true)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddContextTarget("org", 0, "a", "b"))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForKey("org", "a", true).VariationForKey("other", "a", true)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddContextTarget("org", 0, "a").
				AddContextTarget("other", 0, "a"))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForKey("org", "a", true).VariationForKey("org", "a", true)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddContextTarget("org", 0, "a"))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
					VariationIndexForKey("org", "a", 2).VariationIndexForKey("org", "b", 2)
			},
			basicString().On(true).OffVariation(0).FallthroughVariation(2).
				AddContextTarget("org", 2, "a", "b"))

		verifyFlag(t,
			func(f *FlagBuilder) {
				f.VariationForKey("", "a", true).VariationForKey("", "b", true)
			},
			basicBool().On(true).FallthroughVariation(trueVar).
				AddTarget(0, "a", "b").AddContextTarget("user", 0))
	})
}

func TestRuleConfig(t *testing.T) {
	t.Run("simple match returning variation 0/true", func(t *testing.T) {
		matchReturnsVariation0 := basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.ClauseWithKind("user", "name", ldmodel.OperatorIn, ldvalue.String("Lucy")),
			),
		)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch("name", ldvalue.String("Lucy")).ThenReturn(true)
		}, matchReturnsVariation0)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch("name", ldvalue.String("Lucy")).ThenReturnIndex(0)
		}, matchReturnsVariation0)
	})

	t.Run("simple match returning variation 1/false", func(t *testing.T) {
		matchReturnsVariation1 := basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(falseVar).Clauses(
				ldbuilders.ClauseWithKind("user", "name", ldmodel.OperatorIn, ldvalue.String("Lucy")),
			),
		)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch("name", ldvalue.String("Lucy")).ThenReturn(false)
		}, matchReturnsVariation1)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch("name", ldvalue.String("Lucy")).ThenReturnIndex(1)
		}, matchReturnsVariation1)
	})

	t.Run("negated match", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfNotMatch("name", ldvalue.String("Lucy")).ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.Negate(ldbuilders.ClauseWithKind("user", "name", ldmodel.OperatorIn, ldvalue.String("Lucy"))),
			),
		))
	})

	t.Run("multiple clauses", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch("name", ldvalue.String("Lucy")).
				AndMatch("country", ldvalue.String("gb")).
				ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.ClauseWithKind("user", "name", ldmodel.OperatorIn, ldvalue.String("Lucy")),
				ldbuilders.ClauseWithKind("user", "country", ldmodel.OperatorIn, ldvalue.String("gb")),
			),
		))
	})

	t.Run("multiple rules", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch("name", ldvalue.String("Lucy")).ThenReturn(true).
				IfMatch("name", ldvalue.String("Mina")).ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.ClauseWithKind("user", "name", ldmodel.OperatorIn, ldvalue.String("Lucy")),
			),
		).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule1").Variation(trueVar).Clauses(
				ldbuilders.ClauseWithKind("user", "name", ldmodel.OperatorIn, ldvalue.String("Mina")),
			),
		))
	})

	t.Run("simple match with context kind", func(t *testing.T) {
		matchReturnsVariation0 := basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.ClauseWithKind("org", "name", ldmodel.OperatorIn, ldvalue.String("Catco")),
			),
		)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatchContext("org", "name", ldvalue.String("Catco")).ThenReturn(true)
		}, matchReturnsVariation0)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatchContext("org", "name", ldvalue.String("Catco")).ThenReturnIndex(0)
		}, matchReturnsVariation0)
	})

	t.Run("negated match with context kind", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfNotMatchContext("org", "name", ldvalue.String("Catco")).ThenReturn(true)
		}, basicBool().On(true).FallthroughVariation(0).AddRule(
			ldbuilders.NewRuleBuilder().ID("rule0").Variation(trueVar).Clauses(
				ldbuilders.Negate(ldbuilders.ClauseWithKind("org", "name", ldmodel.OperatorIn, ldvalue.String("Catco"))),
			),
		))
	})
}
