package evaluation

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// Notes on some implementation details in this file:
//
// - We are often passing structs by address rather than by value, even if the usual reasons for using
// a pointer (allowing mutation of the value, or using nil to represent "no value") do not apply. This
// is an optimization to avoid the small but nonzero overhead of copying a struct by value across many
// nested function/method calls; passing a pointer instead is faster. It is safe for us to do this
// as long as the pointer value is not being retained outside the scope of this call.
//
// - In some for loops, we are deliberately taking the address of the range variable and using a
// "//nolint:gosec" directive to turn off the usual linter warning about this:
//       for _, x := range someThings {
//           doSomething(&x) //nolint:gosec
//       }
// The rationale is the same as above, and is safe as long as the same conditions apply.

// Result encapsulates all information returned by Evaluator.Evaluate.
type Result struct {
	// Detail contains the evaluation detail fields.
	Detail ldreason.EvaluationDetail

	// IsExperiment is true if this evaluation result was determined by an experiment. Normally if
	// this is true, then Detail.Reason will also communicate that fact, but there are some cases
	// related to the older experimentation model where this field may be true even if Detail.Reason
	// does not say anything special. When the SDK submits evaluation information to the event
	// processor, it should set the RequireReason field in ldevents.FlagEventProperties to this value.
	IsExperiment bool
}

type evaluator struct {
	dataProvider       DataProvider
	bigSegmentProvider BigSegmentProvider
	errorLogger        ldlog.BaseLogger
	enableSecondaryKey bool
}

const ( // See Evaluate() regarding the use of these constants
	preallocatedPrerequisiteChainSize = 20
	preallocatedSegmentChainSize      = 20
)

// NewEvaluator creates an Evaluator, specifying a DataProvider that it will use if it needs to
// query additional feature flags or user segments during an evaluation.
//
// To support big segments, you must use NewEvaluatorWithOptions and EvaluatorOptionBigSegmentProvider.
func NewEvaluator(dataProvider DataProvider) Evaluator {
	return NewEvaluatorWithOptions(dataProvider)
}

// NewEvaluatorWithOptions creates an Evaluator, specifying a DataProvider that it will use if it
// needs to query additional feature flags or user segments during an evaluation, and also
// any number of EvaluatorOption modifiers.
func NewEvaluatorWithOptions(dataProvider DataProvider, options ...EvaluatorOption) Evaluator {
	e := &evaluator{
		dataProvider: dataProvider,
	}
	for _, o := range options {
		if o != nil {
			o.apply(e)
		}
	}
	return e
}

// Used internally to hold the parameters of an evaluation, to avoid repetitive parameter passing.
// Its methods use a pointer receiver for efficiency, even though it is allocated on the stack and
// its fields are never modified.
type evaluationScope struct {
	owner                         *evaluator
	flag                          *ldmodel.FeatureFlag
	context                       ldcontext.Context
	prerequisiteFlagEventRecorder PrerequisiteFlagEventRecorder
	// These bigSegments properties start out unset. They are computed lazily if we encounter
	// big segment references during an evaluation. See evaluator_segment.go.
	bigSegmentsMemberships map[string]BigSegmentMembership
	bigSegmentsStatus      ldreason.BigSegmentsStatus
}

type evaluationStack struct {
	prerequisiteFlagChain []string
	segmentChain          []string
}

// Implementation of the Evaluator interface.
func (e *evaluator) Evaluate(
	flag *ldmodel.FeatureFlag,
	context ldcontext.Context,
	prerequisiteFlagEventRecorder PrerequisiteFlagEventRecorder,
) Result {
	if context.Err() != nil {
		return Result{Detail: ldreason.NewEvaluationDetailForError(ldreason.EvalErrorUserNotSpecified, ldvalue.Null())}
	}

	es := evaluationScope{
		owner:                         e,
		flag:                          flag,
		context:                       context,
		prerequisiteFlagEventRecorder: prerequisiteFlagEventRecorder,
	}

	// Preallocate some space for prerequisiteFlagChain and segmentChain on the stack. We can
	// get up to that many levels of nested prerequisites or nested segments before appending
	// to the slice will cause a heap allocation.
	stack := evaluationStack{
		prerequisiteFlagChain: make([]string, 0, preallocatedPrerequisiteChainSize),
		segmentChain:          make([]string, 0, preallocatedSegmentChainSize),
	}

	detail, _ := es.evaluate(stack)
	if es.bigSegmentsStatus != "" {
		detail.Reason = ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(detail.Reason,
			es.bigSegmentsStatus)
	}
	return Result{Detail: detail, IsExperiment: isExperiment(flag, detail.Reason)}
}

// Entry point for evaluating a flag which could be either the original flag or a prerequisite.
// The second return value is normally true. If it is false, it means we should immediately
// terminate the whole current stack of evaluations and not do any more checking or recursing.
//
// Note that the evaluationStack is passed by value-- unlike other structs such as the FeatureFlag
// which we reference by address for the sake of efficiency (see comments at top of file). One
// reason for this is described in the comments at each point where we modify one of its fields
// with append(). The other is that Go's escape analysis is not quite clever enough to let the
// slices that we preallocated in Evaluate() remain on the stack if we pass that struct by address.
func (es *evaluationScope) evaluate(stack evaluationStack) (ldreason.EvaluationDetail, bool) {
	if !es.flag.On {
		return es.getOffValue(ldreason.NewEvalReasonOff()), true
	}

	prereqErrorReason, ok := es.checkPrerequisites(stack)
	if !ok {
		// Is this an actual error, like a malformed flag? Then return an error with default value.
		if prereqErrorReason.GetKind() == ldreason.EvalReasonError {
			return ldreason.NewEvaluationDetailForError(prereqErrorReason.GetErrorKind(), ldvalue.Null()), false
		}
		// No, it's presumably just "prerequisite failed", which gets the off value.
		return es.getOffValue(prereqErrorReason), true
	}

	// Check to see if targets match
	if variation := es.anyTargetMatchVariation(); variation.IsDefined() {
		return es.getVariation(variation.IntValue(), ldreason.NewEvalReasonTargetMatch()), true
	}

	// Now walk through the rules and see if any match
	for ruleIndex, rule := range es.flag.Rules {
		match, err := es.ruleMatchesContext(&rule, stack) //nolint:gosec // see comments at top of file
		if err != nil {
			es.logEvaluationError(err)
			return ldreason.NewEvaluationDetailForError(errorKindForError(err), ldvalue.Null()), false
		}
		if match {
			reason := ldreason.NewEvalReasonRuleMatch(ruleIndex, rule.ID)
			return es.getValueForVariationOrRollout(rule.VariationOrRollout, reason), true
		}
	}

	return es.getValueForVariationOrRollout(es.flag.Fallthrough, ldreason.NewEvalReasonFallthrough()), true
}

// Do a nested evaluation for a prerequisite of the current scope's flag. The second return value is
// normally true; it is false only in the case where we've detected a circular reference, in which
// case we want the entire evaluation to fail with a MalformedFlag error.
func (es *evaluationScope) evaluatePrerequisite(
	prereqFlag *ldmodel.FeatureFlag,
	stack evaluationStack,
) (ldreason.EvaluationDetail, bool) {
	for _, p := range stack.prerequisiteFlagChain {
		if prereqFlag.Key == p {
			err := circularPrereqReferenceError(prereqFlag.Key)
			es.logEvaluationError(err)
			return ldreason.EvaluationDetail{}, false
		}
	}
	subScope := *es
	subScope.flag = prereqFlag
	result, ok := subScope.evaluate(stack)
	es.bigSegmentsStatus = computeUpdatedBigSegmentsStatus(es.bigSegmentsStatus, subScope.bigSegmentsStatus)
	return result, ok
}

// Returns an empty reason if all prerequisites are OK, otherwise constructs an error reason that describes the failure
func (es *evaluationScope) checkPrerequisites(stack evaluationStack) (ldreason.EvaluationReason, bool) {
	if len(es.flag.Prerequisites) == 0 {
		return ldreason.EvaluationReason{}, true
	}

	stack.prerequisiteFlagChain = append(stack.prerequisiteFlagChain, es.flag.Key)
	// Note that the change to stack.prerequisiteFlagChain does not persist after returning from
	// this method. That means we don't ever need to explicitly remove the last item-- but, it
	// introduces a potential edge-case inefficiency with deeply nested prerequisites: if the
	// original slice had a capacity of 20, and then the 20th prerequisite has 5 prerequisites of
	// its own, when checkPrerequisites is called for each of those it will end up hitting the
	// capacity of the slice each time and allocating a new backing array each time. The way
	// around that would be to pass a *pointer* to the slice, so the backing array would be
	// retained. However, doing so appears to defeat Go's escape analysis and cause heap escaping
	// of the slice every time, which would be worse in more typical use cases. We do not expect
	// the preallocated capacity to be reached in typical usage.

	for _, prereq := range es.flag.Prerequisites {
		prereqFeatureFlag := es.owner.dataProvider.GetFeatureFlag(prereq.Key)
		if prereqFeatureFlag == nil {
			return ldreason.NewEvalReasonPrerequisiteFailed(prereq.Key), false
		}
		prereqOK := true

		prereqResultDetail, prereqValid := es.evaluatePrerequisite(prereqFeatureFlag, stack)
		if !prereqValid {
			// In this case we want to immediately exit with an error and not check any more prereqs
			return ldreason.NewEvalReasonError(ldreason.EvalErrorMalformedFlag), false
		}
		if !prereqFeatureFlag.On || prereqResultDetail.IsDefaultValue() ||
			prereqResultDetail.VariationIndex.IntValue() != prereq.Variation {
			// Note that if the prerequisite flag is off, we don't consider it a match no matter what its
			// off variation was. But we still need to evaluate it in order to generate an event.
			prereqOK = false
		}

		if es.prerequisiteFlagEventRecorder != nil {
			event := PrerequisiteFlagEvent{es.flag.Key, es.context, prereqFeatureFlag, Result{
				Detail:       prereqResultDetail,
				IsExperiment: isExperiment(prereqFeatureFlag, prereqResultDetail.Reason),
			}, prereqFeatureFlag.ExcludeFromSummaries}
			es.prerequisiteFlagEventRecorder(event)
		}

		if !prereqOK {
			return ldreason.NewEvalReasonPrerequisiteFailed(prereq.Key), false
		}
	}
	return ldreason.EvaluationReason{}, true
}

func (es *evaluationScope) getVariation(index int, reason ldreason.EvaluationReason) ldreason.EvaluationDetail {
	if index < 0 || index >= len(es.flag.Variations) {
		err := badVariationError(index)
		es.logEvaluationError(err)
		return ldreason.NewEvaluationDetailForError(err.errorKind(), ldvalue.Null())
	}
	return ldreason.NewEvaluationDetail(es.flag.Variations[index], index, reason)
}

func (es *evaluationScope) getOffValue(reason ldreason.EvaluationReason) ldreason.EvaluationDetail {
	if !es.flag.OffVariation.IsDefined() {
		return ldreason.EvaluationDetail{Reason: reason}
	}
	return es.getVariation(es.flag.OffVariation.IntValue(), reason)
}

func (es *evaluationScope) getValueForVariationOrRollout(
	vr ldmodel.VariationOrRollout,
	reason ldreason.EvaluationReason,
) ldreason.EvaluationDetail {
	index, inExperiment, err := es.variationOrRolloutResult(vr, es.flag.Key, es.flag.Salt)
	if err != nil {
		es.logEvaluationError(err)
		return ldreason.NewEvaluationDetailForError(errorKindForError(err), ldvalue.Null())
	}
	if inExperiment {
		reason = reasonToExperimentReason(reason)
	}
	return es.getVariation(index, reason)
}

func (es *evaluationScope) anyTargetMatchVariation() ldvalue.OptionalInt {
	if len(es.flag.ContextTargets) == 0 {
		// If ContextTargets is empty but Targets is not empty, then this is flag data that originally
		// came from a non-context-aware LD endpoint or SDK. In that case, just look at Targets.
		for _, t := range es.flag.Targets {
			if variation := es.targetMatchVariation(&t); variation.IsDefined() { //nolint:gosec // see comments at top of file
				return variation
			}
		}
	} else {
		// If ContextTargets is provided, we iterate through it-- but, for any target of the default
		// kind (user), if there are no Values, we check for a corresponding target in Targets.
		for _, t := range es.flag.ContextTargets {
			var variation ldvalue.OptionalInt
			if (t.ContextKind == "" || t.ContextKind == ldcontext.DefaultKind) && len(t.Values) == 0 {
				for _, t1 := range es.flag.Targets {
					if t1.Variation == t.Variation {
						variation = es.targetMatchVariation(&t1) //nolint:gosec // see comments at top of file
						break
					}
				}
			} else {
				variation = es.targetMatchVariation(&t) //nolint:gosec // see comments at top of file
			}
			if variation.IsDefined() {
				return variation
			}
		}
	}
	return ldvalue.OptionalInt{}
}

func (es *evaluationScope) targetMatchVariation(t *ldmodel.Target) ldvalue.OptionalInt {
	if context := es.context.IndividualContextByKind(t.ContextKind); context.IsDefined() {
		if ldmodel.EvaluatorAccessors.TargetFindKey(t, context.Key()) {
			return ldvalue.NewOptionalInt(t.Variation)
		}
	}
	return ldvalue.OptionalInt{}
}

func (es *evaluationScope) ruleMatchesContext(rule *ldmodel.FlagRule, stack evaluationStack) (bool, error) {
	// Note that rule is passed by reference only for efficiency; we do not modify it
	for _, clause := range rule.Clauses {
		match, err := es.clauseMatchesContext(&clause, stack) //nolint:gosec // see comments at top of file
		if !match || err != nil {
			return match, err
		}
	}
	return true, nil
}

func (es *evaluationScope) variationOrRolloutResult(
	r ldmodel.VariationOrRollout, key, salt string) (variationIndex int, inExperiment bool, err error) {
	if r.Variation.IsDefined() {
		return r.Variation.IntValue(), false, nil
	}
	if len(r.Rollout.Variations) == 0 {
		// This is an error (malformed flag); either Variation or Rollout must be non-nil.
		return -1, false, emptyRolloutError{}
	}

	isExperiment := r.Rollout.IsExperiment()

	bucketVal, problem, err := es.computeBucketValue(isExperiment, r.Rollout.Seed, r.Rollout.ContextKind,
		key, r.Rollout.BucketBy, salt)
	if err != nil {
		return -1, false, err
	}
	var sum float32

	for _, bucket := range r.Rollout.Variations {
		sum += float32(bucket.Weight) / 100000.0
		if bucketVal < sum {
			resultInExperiment := isExperiment && !bucket.Untracked &&
				problem != bucketingFailureContextLacksDesiredKind
			return bucket.Variation, resultInExperiment, nil
		}
	}

	// The user's bucket value was greater than or equal to the end of the last bucket. This could happen due
	// to a rounding error, or due to the fact that we are scaling to 100000 rather than 99999, or the flag
	// data could contain buckets that don't actually add up to 100000. Rather than returning an error in
	// this case (or changing the scaling, which would potentially change the results for *all* users), we
	// will simply put the user in the last bucket.
	lastBucket := r.Rollout.Variations[len(r.Rollout.Variations)-1]
	return lastBucket.Variation, isExperiment && !lastBucket.Untracked, nil
}

func (es *evaluationScope) logEvaluationError(err error) {
	if err == nil || es.owner.errorLogger == nil {
		return
	}
	es.owner.errorLogger.Printf("Invalid flag configuration detected in flag %q: %s",
		es.flag.Key,
		err,
	)
}

func getApplicableContextKeyByKind(baseContext *ldcontext.Context, kind ldcontext.Kind) (string, bool) {
	if mc := baseContext.IndividualContextByKind(kind); mc.IsDefined() {
		return mc.Key(), true
	}
	return "", false
}

func reasonToExperimentReason(reason ldreason.EvaluationReason) ldreason.EvaluationReason {
	switch reason.GetKind() {
	case ldreason.EvalReasonFallthrough:
		return ldreason.NewEvalReasonFallthroughExperiment(true)
	case ldreason.EvalReasonRuleMatch:
		return ldreason.NewEvalReasonRuleMatchExperiment(reason.GetRuleIndex(), reason.GetRuleID(), true)
	default:
		return reason // COVERAGE: unreachable
	}
}

func isExperiment(flag *ldmodel.FeatureFlag, reason ldreason.EvaluationReason) bool {
	// If the reason says we're in an experiment, we are. Otherwise, apply
	// the legacy rule exclusion logic.
	if reason.IsInExperiment() {
		return true
	}

	switch reason.GetKind() {
	case ldreason.EvalReasonFallthrough:
		return flag.TrackEventsFallthrough
	case ldreason.EvalReasonRuleMatch:
		i := reason.GetRuleIndex()
		if i >= 0 && i < len(flag.Rules) {
			return flag.Rules[i].TrackEvents
		}
	}
	return false
}
