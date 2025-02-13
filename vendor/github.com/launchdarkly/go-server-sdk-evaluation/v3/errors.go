package evaluation

import (
	"fmt"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
)

// These error types are used only internally to distinguish between reasons an evaluation might fail.
// They are surfaced only in terms of the EvaluationReason/ErrorKind types.

// When possible, we define these types as renames of a simple type like string or int, rather than as
// a struct. This is a minor optimization to take advantage of the fact that a simple type that implements
// an interface does not need to be allocated on the heap.

// EvalError is an internal interface for an error that should cause evaluation to fail.
type evalError interface {
	error
	errorKind() ldreason.EvalErrorKind
}

// ErrorKindForError returns the appropriate ldreason.EvalErrorKind value for an error.
func errorKindForError(err error) ldreason.EvalErrorKind {
	if e, ok := err.(evalError); ok {
		return e.errorKind()
	}
	return ldreason.EvalErrorException
}

// BadVariationError means a variation index was out of range. The integer value is the index.
type badVariationError int

func (e badVariationError) Error() string {
	return fmt.Sprintf("rule, fallthrough, or target referenced a nonexistent variation index %d", int(e))
}

func (e badVariationError) errorKind() ldreason.EvalErrorKind {
	return ldreason.EvalErrorMalformedFlag
}

// EmptyAttrRefError means an attribute reference in a clause was undefined
type emptyAttrRefError struct{}

func (e emptyAttrRefError) Error() string {
	return "rule clause did not specify an attribute"
}

func (e emptyAttrRefError) errorKind() ldreason.EvalErrorKind {
	return ldreason.EvalErrorMalformedFlag
}

// BadAttrRefError means an attribute reference in a clause was syntactically invalid. The string value is the
// attribute reference.
type badAttrRefError string

func (e badAttrRefError) Error() string {
	return fmt.Sprintf("invalid attribute reference %q", string(e))
}

func (e badAttrRefError) errorKind() ldreason.EvalErrorKind {
	return ldreason.EvalErrorMalformedFlag
}

// EmptyRolloutError means a rollout or experiment had no variations.
type emptyRolloutError struct{}

func (e emptyRolloutError) Error() string {
	return "rollout or experiment with no variations"
}

func (e emptyRolloutError) errorKind() ldreason.EvalErrorKind {
	return ldreason.EvalErrorMalformedFlag
}

// CircularPrereqReferenceError means there was a cycle in prerequisites. The string value is the key of the
// prerequisite.
type circularPrereqReferenceError string

func (e circularPrereqReferenceError) Error() string {
	return fmt.Sprintf("prerequisite relationship to %q caused a circular reference;"+
		" this is probably a temporary condition due to an incomplete update", string(e))
}

func (e circularPrereqReferenceError) errorKind() ldreason.EvalErrorKind {
	return ldreason.EvalErrorMalformedFlag
}

// CircularSegmentReferenceError means there was a cycle in segment rules. The string value is the key of the
// segment where we detected the cycle.
type circularSegmentReferenceError string

func (e circularSegmentReferenceError) Error() string {
	return fmt.Sprintf("segment rule referencing segment %q caused a circular reference;"+
		" this is probably a temporary condition due to an incomplete update", string(e))
}

// MalformedSegmentError means invalid properties were found while trying to match a segment.
type malformedSegmentError struct {
	SegmentKey string
	Err        error
}

func (e malformedSegmentError) Error() string {
	return fmt.Sprintf("segment %q had an invalid configuration: %s", e.SegmentKey, e.Err)
}

func (e malformedSegmentError) errorKind() ldreason.EvalErrorKind {
	return ldreason.EvalErrorMalformedFlag
	// Technically it's not a malformed *flag*, but we don't have a better error code for this.
}
