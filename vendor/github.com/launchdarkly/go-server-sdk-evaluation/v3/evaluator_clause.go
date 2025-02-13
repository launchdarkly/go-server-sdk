package evaluation

import (
	"strings"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

func (es *evaluationScope) clauseMatchesContext(clause *ldmodel.Clause, stack evaluationStack) (bool, error) {
	// Note that clause is passed by reference only for efficiency; we do not modify it
	// In the case of a segment match operator, we check if the user is in any of the segments,
	// and possibly negate
	if clause.Op == ldmodel.OperatorSegmentMatch {
		for _, value := range clause.Values {
			if value.Type() == ldvalue.StringType {
				if segment := es.owner.dataProvider.GetSegment(value.StringValue()); segment != nil {
					match, err := es.segmentContainsContext(segment, stack)
					if err != nil {
						return false, err
					}
					if match {
						return !clause.Negate, nil // match - true unless negated
					}
				}
			}
		}
		return clause.Negate, nil // non-match - false unless negated
	}

	return clauseMatchesContextNoSegments(clause, &es.context)
}

func clauseMatchesContextNoSegments(c *ldmodel.Clause, context *ldcontext.Context) (bool, error) {
	if !c.Attribute.IsDefined() {
		return false, emptyAttrRefError{}
	}
	if c.Attribute.Err() != nil {
		return false, badAttrRefError(c.Attribute.String())
	}
	if c.Attribute.String() == ldattr.KindAttr {
		return maybeNegate(c.Negate, clauseMatchByKind(c, context)), nil
	}
	actualContext := context.IndividualContextByKind(c.ContextKind)
	if !actualContext.IsDefined() {
		return false, nil
	}
	uValue := actualContext.GetValueForRef(c.Attribute)
	if uValue.IsNull() {
		// if the user attribute is null/missing, it's an automatic non-match - regardless of c.Negate
		return false, nil
	}

	// If the user value is an array, see if the intersection is non-empty. If so, this clause matches
	if uValue.Type() == ldvalue.ArrayType {
		for i := 0; i < uValue.Count(); i++ {
			if matchAny(c, uValue.GetByIndex(i)) {
				return maybeNegate(c.Negate, true), nil
			}
		}
		return maybeNegate(c.Negate, false), nil
	}

	return maybeNegate(c.Negate, matchAny(c, uValue)), nil
}

func maybeNegate(negate, result bool) bool {
	if negate {
		return !result
	}
	return result
}

func matchAny(
	c *ldmodel.Clause,
	value ldvalue.Value,
) bool {
	if c.Op == ldmodel.OperatorIn {
		return ldmodel.EvaluatorAccessors.ClauseFindValue(c, value)
	}
	for i, v := range c.Values {
		if doOp(c, value, v, i) {
			return true
		}
	}
	return false
}

func doOp(c *ldmodel.Clause, ctxValue, clValue ldvalue.Value, index int) bool {
	switch c.Op {
	case ldmodel.OperatorEndsWith:
		return stringOperator(ctxValue, clValue, strings.HasSuffix)
	case ldmodel.OperatorStartsWith:
		return stringOperator(ctxValue, clValue, strings.HasPrefix)
	case ldmodel.OperatorMatches:
		return operatorMatchesFn(c, ctxValue, index)
	case ldmodel.OperatorContains:
		return stringOperator(ctxValue, clValue, strings.Contains)
	case ldmodel.OperatorLessThan:
		return numericOperator(ctxValue, clValue, func(a float64, b float64) bool { return a < b })
	case ldmodel.OperatorLessThanOrEqual:
		return numericOperator(ctxValue, clValue, func(a float64, b float64) bool { return a <= b })
	case ldmodel.OperatorGreaterThan:
		return numericOperator(ctxValue, clValue, func(a float64, b float64) bool { return a > b })
	case ldmodel.OperatorGreaterThanOrEqual:
		return numericOperator(ctxValue, clValue, func(a float64, b float64) bool { return a >= b })
	case ldmodel.OperatorBefore:
		return dateOperator(c, ctxValue, index, time.Time.Before)
	case ldmodel.OperatorAfter:
		return dateOperator(c, ctxValue, index, time.Time.After)
	case ldmodel.OperatorSemVerEqual:
		return semVerOperator(c, ctxValue, index, 0)
	case ldmodel.OperatorSemVerLessThan:
		return semVerOperator(c, ctxValue, index, -1)
	case ldmodel.OperatorSemVerGreaterThan:
		return semVerOperator(c, ctxValue, index, 1)
	}
	return false
}

func clauseMatchByKind(c *ldmodel.Clause, context *ldcontext.Context) bool {
	// If Attribute is "kind", then we treat Operator and Values as a match expression against a list
	// of all individual kinds in the context. That is, for a multi-kind context with kinds of "org"
	// and "user", it is a match if either of those strings is a match with Operator and Values.
	if context.Multiple() {
		for i := 0; i < context.IndividualContextCount(); i++ {
			if individualContext := context.IndividualContextByIndex(i); individualContext.IsDefined() {
				ctxValue := ldvalue.String(string(individualContext.Kind()))
				if matchAny(c, ctxValue) {
					return true
				}
			}
		}
		return false
	}
	ctxValue := ldvalue.String(string(context.Kind()))
	return matchAny(c, ctxValue)
}

func stringOperator(
	ctxValue, clValue ldvalue.Value,
	stringTestFn func(string, string) bool,
) bool {
	if ctxValue.IsString() && clValue.IsString() {
		return stringTestFn(ctxValue.StringValue(), clValue.StringValue())
	}
	return false
}

func operatorMatchesFn(c *ldmodel.Clause, ctxValue ldvalue.Value, clValueIndex int) bool {
	if ctxValue.IsString() {
		r := ldmodel.EvaluatorAccessors.ClauseGetValueAsRegexp(c, clValueIndex)
		if r != nil {
			return r.MatchString(ctxValue.StringValue())
		}
	}
	return false
}

func numericOperator(ctxValue, clValue ldvalue.Value, fn func(float64, float64) bool) bool {
	if ctxValue.IsNumber() && clValue.IsNumber() {
		return fn(ctxValue.Float64Value(), clValue.Float64Value())
	}
	return false
}

func dateOperator(
	c *ldmodel.Clause,
	ctxValue ldvalue.Value,
	clValueIndex int,
	fn func(time.Time, time.Time) bool,
) bool {
	if clValueTime, ok := ldmodel.EvaluatorAccessors.ClauseGetValueAsTimestamp(c, clValueIndex); ok {
		if ctxValueTime, ok := ldmodel.TypeConversions.ValueToTimestamp(ctxValue); ok {
			return fn(ctxValueTime, clValueTime)
		}
	}
	return false
}

func semVerOperator(
	c *ldmodel.Clause,
	ctxValue ldvalue.Value,
	clValueIndex int,
	expectedComparisonResult int,
) bool {
	if clValueVer, ok := ldmodel.EvaluatorAccessors.ClauseGetValueAsSemanticVersion(c, clValueIndex); ok {
		if ctxValueVer, ok := ldmodel.TypeConversions.ValueToSemanticVersion(ctxValue); ok {
			return ctxValueVer.ComparePrecedence(clValueVer) == expectedComparisonResult
		}
	}
	return false
}
