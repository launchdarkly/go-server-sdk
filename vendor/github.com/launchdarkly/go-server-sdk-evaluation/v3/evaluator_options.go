package evaluation

import "github.com/launchdarkly/go-sdk-common/v3/ldlog"

// EvaluatorOption is an optional parameter for NewEvaluator.
type EvaluatorOption interface {
	apply(e *evaluator)
}

type evaluatorOptionBigSegmentProvider struct{ bigSegmentProvider BigSegmentProvider }

// EvaluatorOptionBigSegmentProvider is an option for NewEvaluator that specifies a
// BigSegmentProvider for evaluating big segment membership. If the parameter is nil, it will
// be treated the same as a BigSegmentProvider that always returns a "store not configured"
// status.
func EvaluatorOptionBigSegmentProvider(bigSegmentProvider BigSegmentProvider) EvaluatorOption {
	return evaluatorOptionBigSegmentProvider{bigSegmentProvider: bigSegmentProvider}
}

func (o evaluatorOptionBigSegmentProvider) apply(e *evaluator) {
	e.bigSegmentProvider = o.bigSegmentProvider
}

type evaluatorOptionEnableSecondaryKey struct{ enable bool }

// EvaluatorOptionEnableSecondaryKey is an option for NewEvaluator that specifies whether
// to enable the use of the ldcontext.Secondary meta-attribute in experiments that involve
// rollouts or experiments. By default, this is not enabled in the current Go SDK and Rust
// SDK, and the evaluation engines in other server-side SDKs do not recognize the secondary
// key meta-attribute at all; but the Go and Rust evaluation engines need to be able to
// recognize it when they are doing evaluations involving old-style user data.
func EvaluatorOptionEnableSecondaryKey(enable bool) EvaluatorOption {
	return evaluatorOptionEnableSecondaryKey{enable: enable}
}

func (o evaluatorOptionEnableSecondaryKey) apply(e *evaluator) {
	e.enableSecondaryKey = o.enable
}

type evaluatorOptionErrorLogger struct{ errorLogger ldlog.BaseLogger }

// EvaluatorOptionErrorLogger is an option for NewEvaluator that specifies a logger for
// error reporting. The Evaluator will only log errors for conditions that should not be
// possible and require investigation, such as a malformed flag or a code path that should
// not have been reached. If the parameter is nil, no logging is done.
func EvaluatorOptionErrorLogger(errorLogger ldlog.BaseLogger) EvaluatorOption {
	return evaluatorOptionErrorLogger{errorLogger: errorLogger}
}

func (o evaluatorOptionErrorLogger) apply(e *evaluator) {
	e.errorLogger = o.errorLogger
}
