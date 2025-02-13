package ldbuilders

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// Bucket constructs a WeightedVariation with the specified variation index and weight.
func Bucket(variationIndex int, weight int) ldmodel.WeightedVariation {
	return ldmodel.WeightedVariation{Variation: variationIndex, Weight: weight}
}

// BucketUntracked constructs a WeightedVariation with the specified variation index and weight, where users in this
// bucket will not have tracking events sent.
func BucketUntracked(variationIndex int, weight int) ldmodel.WeightedVariation {
	return ldmodel.WeightedVariation{Variation: variationIndex, Weight: weight, Untracked: true}
}

// Rollout constructs a VariationOrRollout with the specified buckets.
func Rollout(buckets ...ldmodel.WeightedVariation) ldmodel.VariationOrRollout {
	return ldmodel.VariationOrRollout{Rollout: ldmodel.Rollout{Kind: ldmodel.RolloutKindRollout, Variations: buckets}}
}

// Experiment constructs a VariationOrRollout representing an experiment with the specified buckets.
func Experiment(seed ldvalue.OptionalInt, buckets ...ldmodel.WeightedVariation) ldmodel.VariationOrRollout {
	return ldmodel.VariationOrRollout{
		Rollout: ldmodel.Rollout{
			Kind:       ldmodel.RolloutKindExperiment,
			Variations: buckets,
			Seed:       seed,
		},
	}
}

// Variation constructs a VariationOrRollout with the specified variation index.
func Variation(variationIndex int) ldmodel.VariationOrRollout {
	return ldmodel.VariationOrRollout{Variation: ldvalue.NewOptionalInt(variationIndex)}
}

// FlagBuilder provides a builder pattern for FeatureFlag.
type FlagBuilder struct {
	flag ldmodel.FeatureFlag
}

// RuleBuilder provides a builder pattern for FlagRule.
type RuleBuilder struct {
	rule ldmodel.FlagRule
}

// MigrationFlagParametersBuilder provides a builder pattern for MigrationFlagParameter.
type MigrationFlagParametersBuilder struct {
	parameters ldmodel.MigrationFlagParameters
}

// NewFlagBuilder creates a FlagBuilder.
func NewFlagBuilder(key string) *FlagBuilder {
	return &FlagBuilder{flag: ldmodel.FeatureFlag{
		Key:                    key,
		ClientSideAvailability: ldmodel.ClientSideAvailability{UsingMobileKey: true},
	}}
}

// Build returns the configured FeatureFlag.
func (b *FlagBuilder) Build() ldmodel.FeatureFlag {
	f := b.flag
	ldmodel.PreprocessFlag(&f)
	return f
}

// AddPrerequisite adds a flag prerequisite.
func (b *FlagBuilder) AddPrerequisite(key string, variationIndex int) *FlagBuilder {
	b.flag.Prerequisites = append(b.flag.Prerequisites, ldmodel.Prerequisite{Key: key, Variation: variationIndex})
	return b
}

// AddRule adds a flag rule.
func (b *FlagBuilder) AddRule(r *RuleBuilder) *FlagBuilder {
	b.flag.Rules = append(b.flag.Rules, r.Build())
	return b
}

// AddTarget adds a user target set.
func (b *FlagBuilder) AddTarget(variationIndex int, keys ...string) *FlagBuilder {
	b.flag.Targets = append(b.flag.Targets, ldmodel.Target{Values: keys, Variation: variationIndex})
	return b
}

// AddContextTarget adds a target set for any context kind.
func (b *FlagBuilder) AddContextTarget(kind ldcontext.Kind, variationIndex int, keys ...string) *FlagBuilder {
	b.flag.ContextTargets = append(b.flag.ContextTargets,
		ldmodel.Target{ContextKind: kind, Values: keys, Variation: variationIndex})
	return b
}

// ClientSideUsingEnvironmentID sets the flag's ClientSideAvailability.UsingEnvironmentID property.
// By default, this is false. Setting this property also forces the flag to use the newer serialization
// schema so both UsingEnvironmentID and UsingMobileKey will be explicitly specified.
func (b *FlagBuilder) ClientSideUsingEnvironmentID(value bool) *FlagBuilder {
	b.flag.ClientSideAvailability.UsingEnvironmentID = value
	b.flag.ClientSideAvailability.Explicit = true
	return b
}

// ClientSideUsingMobileKey sets the flag's ClientSideAvailability.UsingMobileKey property. By default,
// this is true. Setting this property also forces the flag to use the newer serialization schema so
// both UsingEnvironmentID and UsingMobileKey will be explicitly specified.
func (b *FlagBuilder) ClientSideUsingMobileKey(value bool) *FlagBuilder {
	b.flag.ClientSideAvailability.UsingMobileKey = value
	b.flag.ClientSideAvailability.Explicit = true
	return b
}

// DebugEventsUntilDate sets the flag's DebugEventsUntilDate property.
func (b *FlagBuilder) DebugEventsUntilDate(t ldtime.UnixMillisecondTime) *FlagBuilder {
	b.flag.DebugEventsUntilDate = t
	return b
}

// Deleted sets the flag's Deleted property.
func (b *FlagBuilder) Deleted(value bool) *FlagBuilder {
	b.flag.Deleted = value
	return b
}

// ExcludeFromSummaries sets the flag's ExcludeFromSummaries property.
func (b *FlagBuilder) ExcludeFromSummaries(value bool) *FlagBuilder {
	b.flag.ExcludeFromSummaries = value
	return b
}

// Fallthrough sets the flag's Fallthrough property.
func (b *FlagBuilder) Fallthrough(vr ldmodel.VariationOrRollout) *FlagBuilder {
	b.flag.Fallthrough = vr
	return b
}

// FallthroughVariation sets the flag's Fallthrough property to a fixed variation.
func (b *FlagBuilder) FallthroughVariation(variationIndex int) *FlagBuilder {
	return b.Fallthrough(Variation(variationIndex))
}

// MigrationFlagParameters sets the flag's migration properties to the provided parameter values.
func (b *FlagBuilder) MigrationFlagParameters(parameters ldmodel.MigrationFlagParameters) *FlagBuilder {
	b.flag.Migration = &parameters
	return b
}

// OffVariation sets the flag's OffVariation property.
func (b *FlagBuilder) OffVariation(variationIndex int) *FlagBuilder {
	b.flag.OffVariation = ldvalue.NewOptionalInt(variationIndex)
	return b
}

// On sets the flag's On property.
func (b *FlagBuilder) On(value bool) *FlagBuilder {
	b.flag.On = value
	return b
}

// Salt sets the flag's Salt property.
func (b *FlagBuilder) Salt(value string) *FlagBuilder {
	b.flag.Salt = value
	return b
}

// SamplingRatio configures the 1 in x chance evaluation events will be sampled for this flag.
func (b *FlagBuilder) SamplingRatio(samplingRatio int) *FlagBuilder {
	b.flag.SamplingRatio = ldvalue.NewOptionalInt(samplingRatio)
	return b
}

// SingleVariation configures the flag to have only one variation value which it always returns.
func (b *FlagBuilder) SingleVariation(value ldvalue.Value) *FlagBuilder {
	return b.Variations(value).OffVariation(0).On(false)
}

// TrackEvents sets the flag's TrackEvents property.
func (b *FlagBuilder) TrackEvents(value bool) *FlagBuilder {
	b.flag.TrackEvents = value
	return b
}

// TrackEventsFallthrough sets the flag's TrackEventsFallthrough property.
func (b *FlagBuilder) TrackEventsFallthrough(value bool) *FlagBuilder {
	b.flag.TrackEventsFallthrough = value
	return b
}

// Variations sets the flag's list of variation values.
func (b *FlagBuilder) Variations(values ...ldvalue.Value) *FlagBuilder {
	b.flag.Variations = values
	return b
}

// Version sets the flag's Version property.
func (b *FlagBuilder) Version(value int) *FlagBuilder {
	b.flag.Version = value
	return b
}

// NewRuleBuilder creates a RuleBuilder.
func NewRuleBuilder() *RuleBuilder {
	return &RuleBuilder{}
}

// Build returns the configured FlagRule.
func (b *RuleBuilder) Build() ldmodel.FlagRule {
	return b.rule
}

// Clauses sets the rule's list of clauses.
func (b *RuleBuilder) Clauses(clauses ...ldmodel.Clause) *RuleBuilder {
	b.rule.Clauses = clauses
	return b
}

// ID sets the rule's ID property.
func (b *RuleBuilder) ID(id string) *RuleBuilder {
	b.rule.ID = id
	return b
}

// TrackEvents sets the rule's TrackEvents property.
func (b *RuleBuilder) TrackEvents(value bool) *RuleBuilder {
	b.rule.TrackEvents = value
	return b
}

// Variation sets the rule to use a fixed variation.
func (b *RuleBuilder) Variation(variationIndex int) *RuleBuilder {
	return b.VariationOrRollout(Variation(variationIndex))
}

// VariationOrRollout sets the rule to use either a variation or a percentage rollout.
func (b *RuleBuilder) VariationOrRollout(vr ldmodel.VariationOrRollout) *RuleBuilder {
	b.rule.VariationOrRollout = vr
	return b
}

// Clause constructs a basic Clause. The attr parameter is assumed to be a simple attribute name
// rather than a path reference.
func Clause(attr string, op ldmodel.Operator, values ...ldvalue.Value) ldmodel.Clause {
	return ldmodel.Clause{Attribute: ldattr.NewLiteralRef(attr), Op: op, Values: values}
}

// ClauseWithKind is like Clause, but also specifies a context kind.
func ClauseWithKind(
	contextKind ldcontext.Kind,
	attr string,
	op ldmodel.Operator,
	values ...ldvalue.Value,
) ldmodel.Clause {
	return ldmodel.Clause{
		ContextKind: contextKind,
		Attribute:   ldattr.NewLiteralRef(attr),
		Op:          op,
		Values:      values,
	}
}

// ClauseRef constructs a basic Clause, using the ldattr.Ref type for the attribute reference.
func ClauseRef(attrRef ldattr.Ref, op ldmodel.Operator, values ...ldvalue.Value) ldmodel.Clause {
	return ldmodel.Clause{Attribute: attrRef, Op: op, Values: values}
}

// ClauseRefWithKind is like ClauseRef, but also specifies a context kind.
func ClauseRefWithKind(
	contextKind ldcontext.Kind,
	attrRef ldattr.Ref,
	op ldmodel.Operator,
	values ...ldvalue.Value,
) ldmodel.Clause {
	return ldmodel.Clause{ContextKind: contextKind, Attribute: attrRef, Op: op, Values: values}
}

// Negate returns the same Clause with the Negated property set to true.
func Negate(c ldmodel.Clause) ldmodel.Clause {
	c.Negate = true
	return c
}

// SegmentMatchClause constructs a Clause that uses the segmentMatch operator.
func SegmentMatchClause(segmentKeys ...string) ldmodel.Clause {
	clause := ldmodel.Clause{Op: ldmodel.OperatorSegmentMatch}
	for _, key := range segmentKeys {
		clause.Values = append(clause.Values, ldvalue.String(key))
	}
	return clause
}

// NewMigrationFlagParametersBuilder creates a MigrationFlagParametersBuilder.
func NewMigrationFlagParametersBuilder() *MigrationFlagParametersBuilder {
	return &MigrationFlagParametersBuilder{}
}

// Build returns the configured MigrationFlagParameters.
func (b *MigrationFlagParametersBuilder) Build() ldmodel.MigrationFlagParameters {
	return b.parameters
}

// CheckRatio controls the frequency a consistency check is performed for a migration flag.
func (b *MigrationFlagParametersBuilder) CheckRatio(ratio int) *MigrationFlagParametersBuilder {
	b.parameters.CheckRatio = ldvalue.NewOptionalInt(ratio)
	return b
}
