package ldclient

import (
	"errors"
	"fmt"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/bigsegments"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v6/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v6/ldcomponents/ldstoreimpl"
	"gopkg.in/launchdarkly/go-server-sdk.v6/testhelpers/ldtestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bigSegmentKey = "segmentkey"

// The definition of this function has to be kept in sync with the equivalent function in
// go-server-sdk-evaluation.
func makeBigSegmentRef(segmentKey string, generation int) string {
	return fmt.Sprintf("%s.g%d", segmentKey, generation)
}

func addBigSegmentAndFlag(testData *ldtestdata.TestDataSource) {
	segment := ldbuilders.NewSegmentBuilder(bigSegmentKey).
		Unbounded(true).
		Generation(1).
		Build()
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).On(true).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
		FallthroughVariation(0).
		AddRule(ldbuilders.NewRuleBuilder().Variation(1).Clauses(
			ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segment.Key)),
		)).
		Build()
	testData.UsePreconfiguredSegment(segment)
	testData.UsePreconfiguredFlag(flag)
}

func doBigSegmentsTest(
	t *testing.T,
	action func(client *LDClient, bsStore *sharedtest.MockBigSegmentStore),
) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	testData := ldtestdata.DataSource()
	bsStore := &sharedtest.MockBigSegmentStore{}
	bsStore.TestSetMetadataToCurrentTime()

	addBigSegmentAndFlag(testData)

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = testData
		c.BigSegments = ldcomponents.BigSegments(
			sharedtest.SingleBigSegmentStoreFactory{Store: bsStore},
		)
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	action(client, bsStore)
}

func TestEvalWithBigSegments(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		doBigSegmentsTest(t, func(client *LDClient, bsStore *sharedtest.MockBigSegmentStore) {
			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.BigSegmentsHealthy, detail.Reason.GetBigSegmentsStatus())
		})
	})

	t.Run("user found", func(t *testing.T) {
		doBigSegmentsTest(t, func(client *LDClient, bsStore *sharedtest.MockBigSegmentStore) {
			membership := ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(
				[]string{makeBigSegmentRef(bigSegmentKey, 1)}, nil)
			bsStore.TestSetMembership(bigsegments.HashForUserKey(evalTestUser.GetKey()), membership)

			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.True(t, value)
			assert.Equal(t, ldreason.BigSegmentsHealthy, detail.Reason.GetBigSegmentsStatus())
		})
	})

	t.Run("store error", func(t *testing.T) {
		doBigSegmentsTest(t, func(client *LDClient, bsStore *sharedtest.MockBigSegmentStore) {
			bsStore.TestSetMembershipError(errors.New("sorry"))

			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.BigSegmentsStoreError, detail.Reason.GetBigSegmentsStatus())
		})
	})

	t.Run("store not configured", func(t *testing.T) {
		// deliberately not using a configuration with a Big Segment store here
		withClientEvalTestParams(func(p clientEvalTestParams) {
			addBigSegmentAndFlag(p.data)

			value, detail, err := p.client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.BigSegmentsNotConfigured, detail.Reason.GetBigSegmentsStatus())
		})
	})
}
