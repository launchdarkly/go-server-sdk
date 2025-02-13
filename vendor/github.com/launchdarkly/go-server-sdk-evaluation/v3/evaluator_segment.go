package evaluation

import (
	"fmt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

func makeBigSegmentRef(s *ldmodel.Segment) string {
	// The format of big segment references is independent of what store implementation is being
	// used; the store implementation receives only this string and does not know the details of
	// the data model. The Relay Proxy will use the same format when writing to the store.
	return fmt.Sprintf("%s.g%d", s.Key, s.Generation.IntValue())
}

func (es *evaluationScope) segmentContainsContext(s *ldmodel.Segment, stack evaluationStack) (bool, error) {
	// Have we already visited this segment recursively?
	for _, visitedKey := range stack.segmentChain {
		if visitedKey == s.Key {
			return false, circularSegmentReferenceError(s.Key)
		}
	}

	// Add this segment key to the visited list. Since stack is passed by value, this change does not
	// persist after we return from this method. See comments in evaluationScope.checkPrerequisites().
	stack.segmentChain = append(stack.segmentChain, s.Key)

	// Check if the user is specifically included in or excluded from the segment by key
	if s.Unbounded {
		if !s.Generation.IsDefined() {
			// Big segment queries can only be done if the generation is known. If it's unset,
			// that probably means the data store was populated by an older SDK that doesn't know
			// about the Generation property and therefore dropped it from the JSON data. We'll treat
			// that as a "not configured" condition.
			es.bigSegmentsStatus = ldreason.BigSegmentsNotConfigured
			return false, nil
		}
		// A big segment can only apply to one context kind, so if we don't have a key for that kind,
		// we don't need to bother querying the data.
		key, ok := getApplicableContextKeyByKind(&es.context, s.UnboundedContextKind)
		if !ok {
			return false, nil
		}
		// Even if multiple big segments are referenced within a single flag evaluation, we only need
		// to do this query once per context key, since it returns *all* of the user's segment
		// memberships.
		membership, wasCached := es.bigSegmentsMemberships[key]
		if !wasCached {
			if es.owner.bigSegmentProvider == nil {
				// If the provider is nil, that means the SDK hasn't been configured to be able to
				// use big segments.
				es.bigSegmentsStatus = ldreason.BigSegmentsNotConfigured
			} else {
				var thisQueryStatus ldreason.BigSegmentsStatus
				membership, thisQueryStatus = es.owner.bigSegmentProvider.GetMembership(key)
				// Note that this query is just by key; the context kind doesn't matter because any given
				// Big Segment can only reference one context kind. So if segment A for the "user" kind
				// includes a "user" context with key X, and segment B for the "org" kind includes an "org"
				// context with the same key X, it is fine to say that the membership for key X is
				// segment A and segment B-- there is no ambiguity.
				if es.bigSegmentsMemberships == nil {
					es.bigSegmentsMemberships = make(map[string]BigSegmentMembership)
				}
				es.bigSegmentsMemberships[key] = membership
				es.bigSegmentsStatus = computeUpdatedBigSegmentsStatus(es.bigSegmentsStatus, thisQueryStatus)
			}
		}
		if membership != nil {
			included := membership.CheckMembership(makeBigSegmentRef(s))
			if included.IsDefined() {
				return included.BoolValue(), nil
			}
		}
	} else {
		// always check for included before excluded
		defaultKindKey, hasDefaultKindKey := getApplicableContextKeyByKind(&es.context, ldcontext.DefaultKind)
		isOnlyDefaultKind := es.context.Kind() == ldcontext.DefaultKind
		if hasDefaultKindKey && ldmodel.EvaluatorAccessors.SegmentFindKeyInIncluded(s, defaultKindKey) {
			return true, nil
		}
		if !isOnlyDefaultKind {
			for i := range s.IncludedContexts {
				if es.segmentTargetMatchesContext(&s.IncludedContexts[i]) {
					return true, nil
				}
			}
		}
		if hasDefaultKindKey && ldmodel.EvaluatorAccessors.SegmentFindKeyInExcluded(s, defaultKindKey) {
			return false, nil
		}
		if !isOnlyDefaultKind {
			for i := range s.ExcludedContexts {
				if es.segmentTargetMatchesContext(&s.ExcludedContexts[i]) {
					return false, nil
				}
			}
		}
	}

	// Check if any of the segment rules match
	for _, rule := range s.Rules {
		// Note, taking address of range variable here is OK because it's not used outside the loop
		match, err := es.segmentRuleMatchesContext(&rule, stack, s.Key, s.Salt) //nolint:gosec // see comment above
		if err != nil {
			return false, malformedSegmentError{SegmentKey: s.Key, Err: err}
		}
		if match {
			return true, nil
		}
	}

	return false, nil
}

func (es *evaluationScope) segmentTargetMatchesContext(t *ldmodel.SegmentTarget) bool {
	if key, ok := getApplicableContextKeyByKind(&es.context, t.ContextKind); ok {
		return ldmodel.EvaluatorAccessors.SegmentTargetFindKey(t, key)
	}
	return false
}

func (es *evaluationScope) segmentRuleMatchesContext(
	r *ldmodel.SegmentRule,
	stack evaluationStack,
	key, salt string,
) (bool, error) {
	for i := range r.Clauses {
		// Note that the clause is passed by address only for efficiency; we do not modify it
		match, err := es.clauseMatchesContext(&r.Clauses[i], stack)
		if !match || err != nil {
			return false, err
		}
	}

	// If the Weight is absent, this rule matches
	if !r.Weight.IsDefined() {
		return true, nil
	}

	// All of the clauses are met. Check to see if the user buckets in
	// Note: passing r.RolloutContextKind to computeBucketValue here ensures that 1. we will get any necessary
	// context attributes from the right context if the evaluation context is multi-kind, and 2. if the desired
	// context kind is not available,
	// TEMPORARY - instead of ldcontext.DefaultKind here, we will eventually have a Kind field in the segment
	bucket, failReason, err := es.computeBucketValue(
		false,                 // this is not an experiment
		ldvalue.OptionalInt{}, // seed parameter is only used in experiments, never in segment rollouts
		r.RolloutContextKind,
		key,
		r.BucketBy,
		salt,
	)
	if err != nil {
		// err is only non-nil for problems serious enough to indicate a malformed segment configuration
		return false, err
	}
	if failReason == bucketingFailureContextLacksDesiredKind {
		// This particular bucketing failure condition is specified to cause an automatic non-match for the rule.
		// Other kinds of bucketing failures (such as an unknown bucketBy attribute) do not cause a non-match;
		// they just cause the bucket value to be zero, which in this code path will result in a match. The latter
		// behavior isn't logically consistent, but is preserved for historical reasons since changing it would
		// change existing evaluation results.
		return false, nil
	}
	weight := float32(r.Weight.IntValue()) / 100000.0
	return bucket < weight, nil
}

func computeUpdatedBigSegmentsStatus(old, new ldreason.BigSegmentsStatus) ldreason.BigSegmentsStatus {
	// A single evaluation could end up doing more than one big segments query if there are two different
	// context keys involved. If those queries don't return the same status, we want to make sure we
	// report whichever status is most problematic.
	if old != "" && getBigSegmentsStatusPriority(old) > getBigSegmentsStatusPriority(new) {
		return old
	}
	return new
}

func getBigSegmentsStatusPriority(status ldreason.BigSegmentsStatus) int {
	switch status {
	case ldreason.BigSegmentsStale:
		return 1
	case ldreason.BigSegmentsStoreError:
		return 2
	case ldreason.BigSegmentsNotConfigured:
		// NotConfigured is considered a higher-priority problem than StoreError because it implies that the
		// application can't possibly be working right, whereas StoreError could be a transient database problem.
		return 3
	default:
		return 0
	}
}
