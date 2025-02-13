package ldbuilders

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// SegmentBuilder provides a builder pattern for Segment.
type SegmentBuilder struct {
	segment ldmodel.Segment
}

// SegmentRuleBuilder provides a builder pattern for SegmentRule.
type SegmentRuleBuilder struct {
	rule ldmodel.SegmentRule
}

// NewSegmentBuilder creates a SegmentBuilder.
func NewSegmentBuilder(key string) *SegmentBuilder {
	return &SegmentBuilder{ldmodel.Segment{Key: key}}
}

// Build returns the configured Segment.
func (b *SegmentBuilder) Build() ldmodel.Segment {
	s := b.segment
	ldmodel.PreprocessSegment(&s)
	return s
}

// AddRule adds a rule to the segment.
func (b *SegmentBuilder) AddRule(r *SegmentRuleBuilder) *SegmentBuilder {
	b.segment.Rules = append(b.segment.Rules, r.Build())
	return b
}

// Excluded sets the segment's Excluded list.
func (b *SegmentBuilder) Excluded(keys ...string) *SegmentBuilder {
	b.segment.Excluded = keys
	return b
}

// Included sets the segment's Included list.
func (b *SegmentBuilder) Included(keys ...string) *SegmentBuilder {
	b.segment.Included = keys
	return b
}

// IncludedContextKind adds a target list to the segment's IncludedContexts.
func (b *SegmentBuilder) IncludedContextKind(kind ldcontext.Kind, keys ...string) *SegmentBuilder {
	b.segment.IncludedContexts = append(b.segment.IncludedContexts,
		ldmodel.SegmentTarget{ContextKind: kind, Values: keys})
	return b
}

// ExcludedContextKind adds a target to the segment's ExcludedContexts.
func (b *SegmentBuilder) ExcludedContextKind(kind ldcontext.Kind, keys ...string) *SegmentBuilder {
	b.segment.ExcludedContexts = append(b.segment.ExcludedContexts,
		ldmodel.SegmentTarget{ContextKind: kind, Values: keys})
	return b
}

// Version sets the segment's Version property.
func (b *SegmentBuilder) Version(value int) *SegmentBuilder {
	b.segment.Version = value
	return b
}

// Salt sets the segment's Salt property.
func (b *SegmentBuilder) Salt(value string) *SegmentBuilder {
	b.segment.Salt = value
	return b
}

// Unbounded sets the segment's Unbounded property. "Unbounded segment" is the historical name for
// a big segment.
func (b *SegmentBuilder) Unbounded(value bool) *SegmentBuilder {
	b.segment.Unbounded = value
	return b
}

// UnboundedContextKind sets the segment's UnboundedContextKind property.
func (b *SegmentBuilder) UnboundedContextKind(kind ldcontext.Kind) *SegmentBuilder {
	b.segment.UnboundedContextKind = kind
	return b
}

// Generation sets the segment's Generation property.
func (b *SegmentBuilder) Generation(value int) *SegmentBuilder {
	b.segment.Generation = ldvalue.NewOptionalInt(value)
	return b
}

// NewSegmentRuleBuilder creates a SegmentRuleBuilder.
func NewSegmentRuleBuilder() *SegmentRuleBuilder {
	return &SegmentRuleBuilder{}
}

// Build returns the configured SegmentRule.
func (b *SegmentRuleBuilder) Build() ldmodel.SegmentRule {
	return b.rule
}

// BucketBy sets the rule's BucketBy property. The attr parameter is assumed to be a simple attribute name,
// rather than a path reference.
func (b *SegmentRuleBuilder) BucketBy(attr string) *SegmentRuleBuilder {
	b.rule.BucketBy = ldattr.NewLiteralRef(attr)
	return b
}

// BucketByRef sets the rule's BucketBy property using the ldattr.Ref type.
func (b *SegmentRuleBuilder) BucketByRef(attr ldattr.Ref) *SegmentRuleBuilder {
	b.rule.BucketBy = attr
	return b
}

// Clauses sets the rule's list of clauses.
func (b *SegmentRuleBuilder) Clauses(clauses ...ldmodel.Clause) *SegmentRuleBuilder {
	b.rule.Clauses = clauses
	return b
}

// ID sets the rule's ID property.
func (b *SegmentRuleBuilder) ID(id string) *SegmentRuleBuilder {
	b.rule.ID = id
	return b
}

// RolloutContextKind sets the rule's RolloutContextKind property.
func (b *SegmentRuleBuilder) RolloutContextKind(kind ldcontext.Kind) *SegmentRuleBuilder {
	b.rule.RolloutContextKind = kind
	return b
}

// Weight sets the rule's Weight property.
func (b *SegmentRuleBuilder) Weight(value int) *SegmentRuleBuilder {
	b.rule.Weight = ldvalue.NewOptionalInt(value)
	return b
}
