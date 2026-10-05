package ldclient

// This test demonstrates that the bucketing floating-point precision bug reported in
// https://github.com/launchdarkly/java-core/issues/94 (internal SDK-1537) also affects the Go
// server SDK. It is the Go analogue of java-core's EvaluatorBucketingPrecisionTest and uses the
// identical experiment data, seed, and context key.
//
// The Go evaluation engine (go-server-sdk-evaluation, pinned by this module with no replace
// directive) accumulates rollout bucket boundaries in float32:
//
//	var sum float32
//	for _, bucket := range r.Rollout.Variations {
//	    sum += float32(bucket.Weight) / 100000.0
//	    if bucketVal < sum { ... }
//	}
//
// and divides the context hash by the scale in float32 as well. The FLGEA evaluation spec
// (II.F steps 6d and 7a) requires double-precision division and integer-weight-sum boundaries.
// With a long weight list, the float32 drift shifts a bucket boundary across the context's bucket
// value, flipping its assignment. This is the same defect java-core PR #231 fixes for Java.
//
// Expected to FAIL against the current float32 engine; it passes only once Go adopts the
// double-precision / integer-weight-sum algorithm.

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sdk1537EmptyDataProvider is a no-op provider; the flag under test references no
// prerequisites or segments.
type sdk1537EmptyDataProvider struct{}

func (sdk1537EmptyDataProvider) GetFeatureFlag(string) *ldmodel.FeatureFlag { return nil }
func (sdk1537EmptyDataProvider) GetSegment(string) *ldmodel.Segment         { return nil }

// sdk1537Weights mirrors java-core EvaluatorBucketingPrecisionTest.WEIGHT_DATA verbatim:
// {variation, weight, untracked}. Real experiment data from the issue report; 551 weighted
// variations whose weights sum to exactly 100000.
var sdk1537Weights = []struct {
	variation int
	weight    int
	untracked bool
}{
	{1, 93, true}, {0, 5, false}, {1, 281, true}, {0, 81, false}, {1, 227, true}, {0, 100, false}, {1, 998, true}, {0, 100, false}, {1, 90, true}, {0, 100, false},
	{1, 22, true}, {0, 100, false}, {1, 114, true}, {1, 23, false}, {1, 185, true}, {1, 22, false}, {0, 100, false}, {1, 578, true}, {0, 24, false}, {1, 540, true},
	{0, 51, false}, {1, 163, true}, {1, 100, false}, {1, 279, true}, {1, 2, false}, {1, 500, true}, {0, 100, false}, {1, 498, true}, {1, 56, false}, {1, 210, true},
	{1, 4, false}, {1, 210, true}, {1, 100, false}, {1, 735, true}, {1, 100, false}, {1, 69, true}, {1, 69, false}, {1, 600, true}, {1, 100, false}, {1, 442, true},
	{1, 24, false}, {0, 24, false}, {1, 310, true}, {0, 23, false}, {1, 216, true}, {0, 100, false}, {1, 92, true}, {1, 56, false}, {1, 100, false}, {1, 181, true},
	{0, 59, false}, {1, 727, true}, {0, 100, false}, {1, 17, true}, {0, 66, false}, {1, 394, true}, {1, 32, false}, {1, 139, true}, {0, 92, false}, {1, 155, true},
	{0, 56, false}, {1, 146, true}, {1, 5, false}, {1, 150, true}, {1, 60, false}, {1, 12, false}, {1, 151, true}, {1, 100, false}, {1, 116, true}, {1, 100, false},
	{1, 147, true}, {0, 100, false}, {1, 1591, true}, {0, 68, false}, {1, 290, true}, {0, 17, false}, {1, 163, true}, {0, 20, false}, {1, 120, true}, {1, 39, false},
	{1, 85, false}, {1, 181, true}, {0, 100, false}, {1, 16, true}, {0, 78, false}, {1, 548, true}, {0, 23, false}, {1, 314, true}, {1, 100, false}, {1, 312, true},
	{1, 40, false}, {1, 257, true}, {1, 72, false}, {1, 561, true}, {1, 54, false}, {1, 572, true}, {1, 100, false}, {1, 86, true}, {1, 59, false}, {0, 48, false},
	{1, 466, true}, {1, 91, false}, {1, 836, true}, {1, 15, false}, {1, 206, true}, {0, 100, false}, {1, 1058, true}, {1, 100, false}, {1, 395, true}, {0, 20, false},
	{1, 307, true}, {0, 26, false}, {1, 317, true}, {1, 100, false}, {1, 185, true}, {1, 100, false}, {1, 74, true}, {1, 100, false}, {1, 26, true}, {0, 39, false},
	{0, 100, false}, {1, 499, true}, {1, 16, false}, {1, 138, true}, {0, 13, false}, {1, 774, true}, {1, 100, false}, {1, 43, true}, {1, 4, false}, {1, 498, true},
	{1, 100, false}, {1, 155, true}, {1, 40, false}, {1, 73, false}, {1, 480, true}, {1, 16, false}, {1, 304, true}, {0, 19, false}, {1, 158, true}, {0, 100, false},
	{1, 29, true}, {1, 100, false}, {1, 125, true}, {0, 100, false}, {1, 194, true}, {1, 24, false}, {1, 554, true}, {0, 36, false}, {0, 5, false}, {1, 101, true},
	{1, 13, false}, {0, 100, false}, {1, 365, true}, {0, 100, false}, {1, 232, true}, {1, 21, false}, {1, 191, true}, {0, 100, false}, {1, 328, true}, {1, 7, false},
	{0, 100, false}, {1, 175, true}, {1, 100, false}, {1, 32, true}, {1, 100, false}, {1, 107, true}, {0, 100, false}, {1, 212, true}, {1, 72, false}, {1, 295, true},
	{1, 100, false}, {1, 4, true}, {1, 100, false}, {1, 5, true}, {0, 41, false}, {1, 403, true}, {1, 100, false}, {1, 283, true}, {1, 51, false}, {1, 351, true},
	{0, 100, false}, {1, 1024, true}, {1, 100, false}, {1, 43, true}, {1, 84, false}, {0, 22, false}, {0, 100, false}, {1, 5, true}, {1, 83, false}, {1, 4, false},
	{1, 44, false}, {1, 534, true}, {0, 48, false}, {1, 222, true}, {1, 91, false}, {1, 215, true}, {1, 18, false}, {1, 55, true}, {1, 18, false}, {1, 100, false},
	{1, 279, true}, {0, 100, false}, {1, 382, true}, {0, 11, false}, {1, 535, true}, {0, 100, false}, {1, 226, true}, {0, 100, false}, {1, 27, true}, {0, 100, false},
	{1, 291, true}, {0, 96, false}, {1, 139, true}, {0, 69, false}, {1, 122, true}, {1, 89, false}, {1, 27, false}, {1, 211, true}, {0, 85, false}, {1, 123, true},
	{0, 15, false}, {1, 280, true}, {0, 1, false}, {1, 237, true}, {0, 73, false}, {0, 70, false}, {1, 479, true}, {1, 100, false}, {1, 42, true}, {1, 65, false},
	{0, 11, false}, {1, 143, true}, {0, 34, false}, {1, 201, true}, {1, 60, false}, {1, 922, true}, {1, 100, false}, {1, 363, true}, {1, 80, false}, {1, 100, false},
	{1, 499, true}, {0, 100, false}, {1, 271, true}, {0, 62, false}, {1, 651, true}, {1, 100, false}, {1, 581, true}, {1, 50, false}, {0, 98, false}, {1, 536, true},
	{1, 100, false}, {1, 220, true}, {0, 51, false}, {1, 120, true}, {1, 100, false}, {1, 51, true}, {0, 100, false}, {1, 208, true}, {0, 100, false}, {1, 13, true},
	{1, 8, false}, {0, 100, false}, {1, 141, true}, {0, 100, false}, {1, 556, true}, {1, 25, false}, {1, 248, true}, {0, 20, false}, {1, 346, true}, {0, 100, false},
	{1, 208, true}, {0, 100, false}, {1, 394, true}, {0, 100, false}, {1, 254, true}, {1, 100, false}, {1, 260, true}, {0, 89, false}, {0, 84, false}, {1, 861, true},
	{1, 100, false}, {1, 138, true}, {1, 100, false}, {1, 25, true}, {0, 85, false}, {1, 1226, true}, {0, 5, false}, {1, 816, true}, {1, 100, false}, {1, 224, true},
	{1, 50, false}, {1, 226, true}, {0, 100, false}, {1, 148, true}, {1, 100, false}, {1, 100, true}, {1, 100, false}, {1, 133, true}, {0, 100, false}, {1, 471, true},
	{1, 100, false}, {1, 636, true}, {1, 100, false}, {1, 48, true}, {1, 31, false}, {1, 254, true}, {0, 11, false}, {1, 187, true}, {0, 100, false}, {1, 7, true},
	{0, 42, false}, {1, 847, true}, {0, 100, false}, {1, 16, true}, {1, 100, false}, {1, 305, true}, {1, 100, false}, {1, 888, true}, {1, 84, false}, {1, 947, true},
	{1, 8, false}, {1, 19, false}, {1, 907, true}, {0, 100, false}, {1, 449, true}, {1, 38, false}, {1, 64, false}, {1, 1125, true}, {1, 8, false}, {0, 100, false},
	{1, 895, true}, {0, 100, false}, {1, 137, true}, {1, 100, false}, {1, 186, true}, {0, 100, false}, {1, 402, true}, {1, 59, false}, {1, 5, false}, {1, 80, true},
	{1, 82, false}, {1, 480, true}, {1, 26, false}, {1, 94, true}, {0, 100, false}, {1, 89, true}, {0, 100, false}, {1, 387, true}, {1, 100, false}, {1, 271, true},
	{1, 26, false}, {0, 36, false}, {1, 833, true}, {1, 73, false}, {1, 397, true}, {1, 100, false}, {1, 509, true}, {0, 100, false}, {1, 183, true}, {0, 17, false},
	{1, 126, true}, {1, 30, false}, {1, 370, true}, {1, 20, false}, {1, 100, false}, {1, 58, true}, {0, 18, false}, {1, 222, true}, {1, 100, false}, {1, 238, true},
	{1, 80, false}, {0, 100, false}, {1, 97, true}, {1, 60, false}, {1, 386, true}, {1, 2, false}, {1, 100, false}, {1, 433, true}, {1, 100, false}, {1, 21, true},
	{0, 42, false}, {1, 609, true}, {0, 100, false}, {1, 52, true}, {0, 46, false}, {1, 103, true}, {1, 100, false}, {1, 1566, true}, {0, 35, false}, {1, 220, true},
	{1, 40, false}, {1, 553, true}, {1, 100, false}, {1, 39, true}, {0, 71, false}, {1, 75, true}, {1, 100, false}, {1, 132, true}, {0, 100, false}, {1, 91, true},
	{1, 12, false}, {0, 100, false}, {1, 163, true}, {0, 41, false}, {1, 289, true}, {0, 1, false}, {1, 831, true}, {1, 6, false}, {1, 358, true}, {0, 100, false},
	{1, 109, true}, {1, 93, false}, {0, 85, false}, {1, 300, true}, {0, 100, false}, {1, 14, true}, {0, 26, false}, {1, 2320, true}, {0, 100, false}, {1, 202, true},
	{0, 93, false}, {1, 141, true}, {1, 39, false}, {1, 246, true}, {0, 68, false}, {1, 381, true}, {0, 33, false}, {1, 733, true}, {1, 60, false}, {1, 191, true},
	{1, 100, false}, {1, 240, true}, {1, 8, false}, {1, 597, true}, {1, 35, false}, {1, 125, true}, {1, 71, false}, {1, 132, true}, {1, 45, false}, {1, 366, true},
	{1, 59, false}, {0, 25, false}, {1, 163, true}, {1, 16, false}, {1, 273, true}, {1, 1, false}, {0, 100, false}, {1, 57, true}, {0, 77, false}, {1, 179, true},
	{1, 100, false}, {1, 47, true}, {1, 60, false}, {1, 950, true}, {1, 22, false}, {1, 887, true}, {1, 100, false}, {1, 681, true}, {1, 31, false}, {1, 206, true},
	{1, 100, false}, {1, 301, true}, {0, 100, false}, {1, 54, true}, {1, 100, false}, {1, 23, true}, {0, 100, false}, {1, 549, true}, {0, 100, false}, {1, 100, false},
	{1, 193, true}, {0, 100, false}, {1, 63, true}, {1, 59, false}, {1, 345, true}, {0, 100, false}, {1, 3, true}, {1, 86, false}, {1, 2, false}, {1, 279, true},
	{1, 100, false}, {1, 445, true}, {0, 13, false}, {0, 100, false}, {1, 18, true}, {1, 24, false}, {1, 35, false}, {0, 100, false}, {1, 213, true}, {0, 100, false},
	{1, 325, true}, {0, 100, false}, {1, 2, true}, {0, 100, false}, {1, 842, true}, {1, 100, false}, {1, 46, true}, {0, 100, false}, {1, 221, true}, {1, 100, false},
	{1, 74, true}, {1, 25, false}, {1, 211, true}, {0, 29, false}, {0, 100, false}, {1, 13, true}, {0, 100, false}, {1, 90, true}, {1, 10, false}, {0, 19, false},
	{0, 13, false}, {1, 132, true}, {0, 100, false}, {1, 185, true}, {1, 32, false}, {1, 176, true}, {0, 100, false}, {1, 455, true}, {1, 6, false}, {0, 11, false},
	{1, 399, true}, {1, 13, false}, {1, 315, true}, {1, 44, false}, {1, 100, false}, {1, 425, true}, {1, 90, false}, {1, 30, false}, {0, 3, false}, {1, 116, true},
	{1, 67, false}, {1, 306, true}, {1, 100, false}, {1, 53, true}, {0, 100, false}, {1, 1183, true}, {0, 23, false}, {1, 259, true}, {0, 100, false}, {1, 159, true},
	{0, 27, false}, {1, 451, true}, {1, 24, false}, {1, 87, false}, {0, 100, false}, {1, 109, true}, {1, 100, false}, {1, 42, true}, {0, 100, false}, {1, 78, true},
	{0, 32, false},
}

func TestSDK1537BucketingPrecisionAffectsGoServer(t *testing.T) {
	const seed = 682385145
	const contextKey = "2937330902736791534808"

	// Sanity check on the dataset: the issue's weights sum to exactly 100000.
	buckets := make([]ldmodel.WeightedVariation, 0, len(sdk1537Weights))
	total := 0
	for _, w := range sdk1537Weights {
		total += w.weight
		if w.untracked {
			buckets = append(buckets, ldbuilders.BucketUntracked(w.variation, w.weight))
		} else {
			buckets = append(buckets, ldbuilders.Bucket(w.variation, w.weight))
		}
	}
	require.Equal(t, 551, len(buckets))
	require.Equal(t, 100000, total)

	flag := ldbuilders.NewFlagBuilder("test").
		On(true).
		Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		Fallthrough(ldbuilders.Experiment(ldvalue.NewOptionalInt(seed), buckets...)).
		Build()

	evaluator := ldeval.NewEvaluator(sdk1537EmptyDataProvider{})
	result := evaluator.Evaluate(&flag, ldcontext.New(contextKey), nil)

	idx := result.Detail.VariationIndex.IntValue()
	inExp := result.Detail.Reason.IsInExperiment()
	t.Logf("Go server SDK evaluation: variation index=%d, inExperiment=%v (Result.IsExperiment=%v)",
		idx, inExp, result.IsExperiment)

	// Spec-correct behavior (double precision + integer weight sums, per FLGEA II.F steps 6d/7a,
	// as implemented by java-core PR #231): the context's bucket value is 0.9830894514485481, which
	// is below the cumulative-weight boundary 98309/100000 = 0.98309. It therefore belongs to the
	// 536th weighted variation (0-based index 535: {variation 1, weight 1183, untracked}) and is
	// NOT in the experiment.
	//
	// The Go engine accumulates the weight boundaries in float32, drifting the boundary below the
	// bucket value and wrongly advancing the context into the next bucket (index 536:
	// {variation 0, weight 23, tracked}) -> variation 0, inExperiment=true. These assertions fail,
	// demonstrating that SDK-1537 / java-core#94 affects the Go server SDK.
	assert.Equal(t, 1, idx,
		"spec-correct variation index is 1 (the untracked bucket); float32 drift yields 0")
	assert.False(t, inExp,
		"spec-correct result is NOT in the experiment; float32 drift makes Go report inExperiment=true")
}
