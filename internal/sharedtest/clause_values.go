package sharedtest

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// ManyInClauseValues returns enough values that an "in" clause with them gets a lookup set. If the
// SDK releases clause values, such a clause does not keep its Values list.
func ManyInClauseValues() []ldvalue.Value {
	return []ldvalue.Value{
		ldvalue.String("a"), ldvalue.String("b"), ldvalue.String("c"), ldvalue.String("d"),
		ldvalue.String("e"), ldvalue.String("f"), ldvalue.String("g"), ldvalue.String("h"),
	}
}

// FlagWithManyInClauseValues returns a flag with one rule. The rule has an "in" clause on the context
// key with the values of ManyInClauseValues, and it returns variation 0. Otherwise the flag returns
// variation 1.
func FlagWithManyInClauseValues(key string, version int) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(version).On(true).
		Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		AddRule(ldbuilders.NewRuleBuilder().ID("r").Variation(0).
			Clauses(ldbuilders.Clause("key", ldmodel.OperatorIn, ManyInClauseValues()...))).
		FallthroughVariation(1).
		Build()
}

// SegmentWithManyInClauseValues returns a segment with one rule. The rule has an "in" clause on the
// context key with the values of ManyInClauseValues.
func SegmentWithManyInClauseValues(key string, version int) ldmodel.Segment {
	return ldbuilders.NewSegmentBuilder(key).Version(version).
		AddRule(ldbuilders.NewSegmentRuleBuilder().
			Clauses(ldbuilders.Clause("key", ldmodel.OperatorIn, ManyInClauseValues()...))).
		Build()
}

// FirstClauseValues returns the Values list of the first clause in the first rule of the flag.
func FirstClauseValues(flag *ldmodel.FeatureFlag) []ldvalue.Value {
	return flag.Rules[0].Clauses[0].Values
}

// FirstSegmentClauseValues returns the Values list of the first clause in the first rule of the
// segment.
func FirstSegmentClauseValues(segment *ldmodel.Segment) []ldvalue.Value {
	return segment.Rules[0].Clauses[0].Values
}
