package evaluation

import (
	"crypto/sha1" //nolint:gosec // SHA1 is cryptographically weak but we are not using it to hash any credentials
	"encoding/hex"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/internal"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const (
	longScale = float32(0xFFFFFFFFFFFFFFF)

	initialHashInputBufferSize = 100
)

type bucketingFailureReason int

const (
	bucketingFailureInvalidAttrRef bucketingFailureReason = iota + 1 // 0 means no failure
	bucketingFailureContextLacksDesiredKind
	bucketingFailureAttributeNotFound
	bucketingFailureAttributeValueWrongType
)

// computeBucketValue is used for rollouts and experiments in flag rules, flag fallthroughs, and segment rules--
// anywhere a rollout/experiment can be. It implements the logic in the flag evaluation spec for computing a
// one-way hash from some combination of inputs related to the context and the flag or segment, and converting
// that hash into a percentage represented as a floating-point value in the range [0,1].
//
// The isExperiment parameter is true if this is an experiment rather than a plain rollout. Experiments can use
// the seed parameter in place of the context key and flag key; rollouts cannot. Rollouts can use the attr
// parameter to specify a context attribute other than the key, and can include a context's "secondary" key in
// the inputs; experiments cannot. Parameters that are irrelevant in either case are simply ignored.
//
// There are several conditions that could cause this computation to fail. The only one that causes an actual
// error value to be returned is if there is an invalid attribute reference, since that indicates malformed
// flag/segment data. For all other failure conditions, the method returns a zero bucket value, plus an enum
// indicating the type of failure (since these may have somewhat different consequences in different areas of
// evaluations).
func (es *evaluationScope) computeBucketValue(
	isExperiment bool,
	seed ldvalue.OptionalInt,
	contextKind ldcontext.Kind,
	key string,
	attr ldattr.Ref,
	salt string,
) (float32, bucketingFailureReason, error) {
	hashInput := internal.LocalBuffer{Data: make([]byte, 0, initialHashInputBufferSize)}
	// As long as the total length of the append operations below doesn't exceed the initial size,
	// this byte slice will stay on the stack. But since some of the data we're appending comes from
	// context attributes created by the application, we can't rule out that they will be longer than
	// that, in which case the buffer is reallocated automatically.

	if seed.IsDefined() {
		hashInput.AppendInt(seed.IntValue())
	} else {
		hashInput.AppendString(key)
		hashInput.AppendByte('.')
		hashInput.AppendString(salt)
	}
	hashInput.AppendByte('.')

	if isExperiment || !attr.IsDefined() { // always bucket by key in an experiment
		attr = ldattr.NewLiteralRef(ldattr.KeyAttr)
	} else if attr.Err() != nil {
		return 0, bucketingFailureInvalidAttrRef, badAttrRefError(attr.String())
	}
	selectedContext := es.context.IndividualContextByKind(contextKind)
	if !selectedContext.IsDefined() {
		return 0, bucketingFailureContextLacksDesiredKind, nil
	}
	uValue := selectedContext.GetValueForRef(attr)
	if uValue.IsNull() { // attributes can't be null, so null means it doesn't exist
		return 0, bucketingFailureAttributeNotFound, nil
	}
	switch {
	case uValue.IsString():
		hashInput.AppendString(uValue.StringValue())
	case uValue.IsInt():
		hashInput.AppendInt(uValue.IntValue())
	default:
		// Non-integer numbers, and values of any other JSON type, can't be used for bucketing because they have no
		// single reliable representation as a string.
		return 0, bucketingFailureAttributeValueWrongType, nil
	}

	if es.owner.enableSecondaryKey && !isExperiment { // secondary key is not supported in experiments
		if secondary := selectedContext.Secondary(); secondary.IsDefined() { //nolint:staticcheck
			// the nolint directive is because we're deliberately referencing the deprecated Secondary
			hashInput.AppendByte('.')
			hashInput.AppendString(secondary.StringValue())
		}
	}

	hashOutputBytes := sha1.Sum(hashInput.Data) //nolint:gas // just used for insecure hashing
	hexEncodedChars := make([]byte, 64)
	hex.Encode(hexEncodedChars, hashOutputBytes[:])
	hash := hexEncodedChars[:15]

	intVal, _ := internal.ParseHexUint64(hash)

	bucket := float32(intVal) / longScale

	return bucket, 0, nil
}
