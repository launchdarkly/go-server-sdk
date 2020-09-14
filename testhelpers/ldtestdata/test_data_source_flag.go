package ldtestdata

import (
	"fmt"
	"sort"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

const (
	trueVariationForBool  = 0
	falseVariationForBool = 1
)

// FlagBuilder is a builder for feature flag configurations to be used with TestDataSource.
type FlagBuilder struct {
	key                  string
	on                   bool
	offVariation         ldvalue.OptionalInt
	fallthroughVariation ldvalue.OptionalInt
	variations           []ldvalue.Value
	targets              map[int]map[string]bool
	rules                []*RuleBuilder
}

// RuleBuilder is a builder for feature flag rules to be used with TestDataSource.
//
// In the LaunchDarkly model, a flag can have any number of rules, and a rule can have any number of
// clauses. A clause is an individual test such as "name is 'X'". A rule matches a user if all of the
// rule's clauses match the user.
//
// To start defining a rule, use one of the flag builder's matching methods such as IfMatch. This
// defines the first clause for the rule. Optionally, you may add more clauses with the rule builder's
// methods such as AndMatch. Finally, call ThenReturn or ThenReturnIndex to finish defining the rule.
type RuleBuilder struct {
	owner     *FlagBuilder
	variation int
	clauses   []ldmodel.Clause
}

func newFlagBuilder(key string) *FlagBuilder {
	return &FlagBuilder{
		key: key,
		on:  true,
	}
}

func copyFlagBuilder(from *FlagBuilder) *FlagBuilder {
	f := new(FlagBuilder)
	*f = *from
	f.variations = make([]ldvalue.Value, len(from.variations))
	copy(f.variations, from.variations)
	if f.rules != nil {
		f.rules = make([]*RuleBuilder, 0, len(from.rules))
		for _, r := range from.rules {
			f.rules = append(f.rules, copyTestFlagRuleBuilder(r, f))
		}
	}
	if f.targets != nil {
		f.targets = make(map[int]map[string]bool, len(from.targets))
		for v, fromKeys := range from.targets {
			keys := make(map[string]bool, len(fromKeys))
			for k := range fromKeys {
				keys[k] = true
			}
			f.targets[v] = keys
		}
	}
	return f
}

// BooleanFlag is a shortcut for setting the flag to use the standard boolean configuration.
//
// This is the default for all new flags created with TestDataSource.Flag. The flag will have two
// variations, true and false (in that order); it will return false whenever targeting is off, and
// true when targeting is on if no other settings specify otherwise.
func (f *FlagBuilder) BooleanFlag() *FlagBuilder {
	if f.isBooleanFlag() {
		return f
	}
	return f.Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		FallthroughVariationIndex(trueVariationForBool).
		OffVariationIndex(falseVariationForBool)
}

// On sets targeting to be on or off for this flag.
//
// The effect of this depends on the rest of the flag configuration, just as it does on the
// real LaunchDarkly dashboard. In the default configuration that you get from calling
// TestDataSource.Flag with a new flag key, the flag will return false whenever targeting is
// off, and true when targeting is on.
func (f *FlagBuilder) On(on bool) *FlagBuilder {
	f.on = on
	return f
}

// FallthroughVariation specifies the fallthrough variation for a boolean flag. The fallthrough is
// the value that is returned if targeting is on and the user was not matched by a more specific
// target or rule.
//
// If the flag was previously configured with other variations, this also changes it to a boolean
// boolean flag.
//
// To specify the variation by variation index instead (such as for a non-boolean flag), use
// FallthroughVariationIndex.
func (f *FlagBuilder) FallthroughVariation(variation bool) *FlagBuilder {
	return f.BooleanFlag().FallthroughVariationIndex(variationForBool(variation))
}

// FallthroughVariationIndex specifies the index of the fallthrough variation. The fallthrough is
// the value that is returned if targeting is on and the user was not matched by a more specific
// target or rule. The index is 0 for the first variation, 1 for the second, etc.
//
// To specify the variation as true or false instead, for a boolean flag, use
// FallthroughVariation.
func (f *FlagBuilder) FallthroughVariationIndex(variationIndex int) *FlagBuilder {
	f.fallthroughVariation = ldvalue.NewOptionalInt(variationIndex)
	return f
}

// OffVariation specifies the off variation for a boolean flag. This is the variation that is
// returned whenever targeting is off.
//
// If the flag was previously configured with other variations, this also changes it to a boolean
// boolean flag.
//
// To specify the variation by variation index instead (such as for a non-boolean flag), use
// OffVariationIndex.
func (f *FlagBuilder) OffVariation(variation bool) *FlagBuilder {
	return f.BooleanFlag().OffVariationIndex(variationForBool(variation))
}

// OffVariationIndex specifies the index of the off variation. This is the variation that is
// returned whenever targeting is off. The index is 0 for the first variation, 1 for the second, etc.
//
// To specify the variation as true or false instead, for a boolean flag, use
// OffVariation.
func (f *FlagBuilder) OffVariationIndex(variationIndex int) *FlagBuilder {
	f.offVariation = ldvalue.NewOptionalInt(variationIndex)
	return f
}

// VariationForAllUsers sets the flag to return the specified boolean variation by default for all users.
//
// Targeting is switched on, any existing targets or rules are removed, and the flag's variations are
// set to true and false. The fallthrough variation is set to the specified value. The off variation is
// left unchanged.
//
// To specify the variation by variation index instead (such as for a non-boolean flag), use
// VariationForAllUsersIndex.
func (f *FlagBuilder) VariationForAllUsers(variation bool) *FlagBuilder {
	return f.BooleanFlag().VariationForAllUsersIndex(variationForBool(variation))
}

// VariationForAllUsersIndex sets the flag to always return the specified variation for all users.
// The index is 0 for the first variation, 1 for the second, etc.
//
// Targeting is switched on, and any existing targets or rules are removed. The fallthrough variation
// is set to the specified value. The off variation is left unchanged.
//
// To specify the variation as true or false instead, for a boolean flag, use
// VariationForAllUsers.
func (f *FlagBuilder) VariationForAllUsersIndex(variationIndex int) *FlagBuilder {
	return f.On(true).ClearRules().ClearUserTargets().FallthroughVariationIndex(variationIndex)
}

// ValueForAllUsers sets the flag to always return the specified variation value for all users.
//
// The value may be of any JSON type, as defined by ldvalue.Value. This method changes the flag to
// only a single variation, which is this value, and to return the same variation regardless of
// whether targeting is on or off. Any existing targets or rules are removed.
func (f *FlagBuilder) ValueForAllUsers(value ldvalue.Value) *FlagBuilder {
	f.variations = []ldvalue.Value{value}
	return f.VariationForAllUsersIndex(0)
}

// VariationForUser sets the flag to return the specified boolean variation for a specific user key
// when targeting is on.
//
// This has no effect when targeting is turned off for the flag.
//
// If the flag was not already a boolean flag, this also changes it to a boolean flag.
//
// To specify the variation by variation index instead (such as for a non-boolean flag), use
// VariationIndexForUser.
func (f *FlagBuilder) VariationForUser(userKey string, variation bool) *FlagBuilder {
	return f.BooleanFlag().VariationIndexForUser(userKey, variationForBool(variation))
}

// VariationIndexForUser sets the flag to return the specified variation for a specific user key when
// targeting is on. The index is 0 for the first variation, 1 for the second, etc.
//
// This has no effect when targeting is turned off for the flag.
//
// If the flag was not already a boolean flag, this also changes it to a boolean flag.
//
// To specify the variation by variation index instead (such as for a non-boolean flag), use
// VariationIndexForUser.
func (f *FlagBuilder) VariationIndexForUser(userKey string, variationIndex int) *FlagBuilder {
	if f.targets == nil {
		f.targets = make(map[int]map[string]bool)
	}
	for i := range f.variations {
		keys := f.targets[i]
		if i == variationIndex {
			if keys == nil {
				keys = make(map[string]bool)
				f.targets[i] = keys
			}
			keys[userKey] = true
		} else {
			delete(keys, userKey)
		}
	}
	return f
}

// Variations changes the allowable variation values for the flag.
//
// The values may be of any JSON type, as defined by ldvalue.LDValue. For instance, a boolean flag
// normally has ldvalue.Bool(true), ldvalue.Bool(false); a string-valued flag might have
// ldvalue.String("red"), ldvalue.String("green")}; etc.
func (f *FlagBuilder) Variations(values ...ldvalue.Value) *FlagBuilder {
	f.variations = make([]ldvalue.Value, len(values))
	copy(f.variations, values)
	return f
}

// IfMatch starts defining a flag rule, using the "is one of" operator.
//
// The method returns a RuleBuilder. Call its ThenReturn or ThenReturnIndex method to finish
// the rule, or add more tests with another method like AndMatch.
//
// For example, this creates a rule that returns true if the name is "Patsy" or "Edina":
//
//     testData.Flag("flag").
//         IfMatch(lduser.NameAttribute, ldvalue.String("Patsy"), ldvalue.String("Edina")).
//             ThenReturn(true)
func (f *FlagBuilder) IfMatch(attribute lduser.UserAttribute, values ...ldvalue.Value) *RuleBuilder {
	return newTestFlagRuleBuilder(f).AndMatch(attribute, values...)
}

// IfNotMatch starts defining a flag rule, using the "is not one of" operator.
//
// The method returns a RuleBuilder. Call its ThenReturn or ThenReturnIndex method to finish
// the rule, or add more tests with another method like AndMatch.
//
// For example, this creates a rule that returns true if the name is neither "Saffron" nor "Bubble":
//
//     testData.Flag("flag").
//         IfNotMatch(lduser.NameAttribute, ldvalue.String("Saffron"), ldvalue.String("Bubble")).
//         ThenReturn(true)
func (f *FlagBuilder) IfNotMatch(attribute lduser.UserAttribute, values ...ldvalue.Value) *RuleBuilder {
	return newTestFlagRuleBuilder(f).AndNotMatch(attribute, values...)
}

// ClearRules removes any existing rules from the flag. This undoes the effect of methods like
// IfMatch.
func (f *FlagBuilder) ClearRules() *FlagBuilder {
	f.rules = nil
	return f
}

// ClearUserTargets removes any existing user targets from the flag. This undoes the effect of methods
// like VariationForUser.
func (f *FlagBuilder) ClearUserTargets() *FlagBuilder {
	f.targets = nil
	return f
}

func (f *FlagBuilder) isBooleanFlag() bool {
	return len(f.variations) == 2 &&
		f.variations[trueVariationForBool].Equal(ldvalue.Bool(true)) &&
		f.variations[falseVariationForBool].Equal(ldvalue.Bool(false))
}

func (f *FlagBuilder) createFlag(version int) ldmodel.FeatureFlag {
	fb := ldbuilders.NewFlagBuilder(f.key).
		Version(version).
		On(f.on).
		Variations(f.variations...)
	if f.offVariation.IsDefined() {
		fb.OffVariation(f.offVariation.IntValue())
	}
	if f.fallthroughVariation.IsDefined() {
		fb.FallthroughVariation(f.fallthroughVariation.IntValue())
	}

	// We iterate through the target list in variation index order, and sort the user keys in each target,
	// for the sake of test determinacy
	for v := range f.variations {
		if keysMap, ok := f.targets[v]; ok {
			keys := make([]string, 0, len(keysMap))
			for key := range keysMap {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			fb.AddTarget(v, keys...)
		}
	}
	for i, r := range f.rules {
		fb.AddRule(ldbuilders.NewRuleBuilder().
			ID(fmt.Sprintf("rule%d", i)).
			Variation(r.variation).
			Clauses(r.clauses...),
		)
	}
	return fb.Build()
}

func newTestFlagRuleBuilder(owner *FlagBuilder) *RuleBuilder {
	return &RuleBuilder{owner: owner}
}

func copyTestFlagRuleBuilder(from *RuleBuilder, owner *FlagBuilder) *RuleBuilder {
	r := RuleBuilder{owner: owner, variation: from.variation}
	r.clauses = make([]ldmodel.Clause, len(from.clauses))
	copy(r.clauses, from.clauses)
	return &r
}

// AndMatch adds another clause, using the "is one of" operator.
//
// For example, this creates a rule that returns true if the name is "Patsy" and the country is "gb":
//
//     testData.Flag("flag").
//         IfMatch(lduser.NameAttribute, ldvalue.String("Patsy")).
//             AndMatch(lduser.CountryAttribute, ldvalue.String("gb")).
//             ThenReturn(true)
func (r *RuleBuilder) AndMatch(attribute lduser.UserAttribute, values ...ldvalue.Value) *RuleBuilder {
	r.clauses = append(r.clauses, ldbuilders.Clause(attribute, ldmodel.OperatorIn, values...))
	return r
}

// AndNotMatch adds another clause, using the "is not one of" operator.
//
// For example, this creates a rule that returns true if the name is "Patsy" and the country is not "gb":
//
//     testData.Flag("flag").
//         IfMatch(lduser.NameAttribute, ldvalue.String("Patsy")).
//             AndNotMatch(lduser.CountryAttribute, ldvalue.String("gb")).
//             ThenReturn(true)
func (r *RuleBuilder) AndNotMatch(
	attribute lduser.UserAttribute,
	values ...ldvalue.Value,
) *RuleBuilder {
	r.clauses = append(r.clauses, ldbuilders.Negate(ldbuilders.Clause(attribute, ldmodel.OperatorIn, values...)))
	return r
}

// ThenReturn finishes defining the rule, specifying the result value as a boolean.
func (r *RuleBuilder) ThenReturn(variation bool) *FlagBuilder {
	r.owner.BooleanFlag()
	return r.ThenReturnIndex(variationForBool(variation))
}

// ThenReturnIndex finishes defining the rule, specifying the result as a variation index. The index
// is 0 for the first variation, 1 for the second, etc.
func (r *RuleBuilder) ThenReturnIndex(variation int) *FlagBuilder {
	r.variation = variation
	r.owner.rules = append(r.owner.rules, r)
	return r.owner
}

func variationForBool(value bool) int {
	if value {
		return trueVariationForBool
	}
	return falseVariationForBool
}
