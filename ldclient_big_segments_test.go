package ldclient

import (
	"errors"
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/bigsegments"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"

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
	action func(client *LDClient, bsStore *mocks.MockBigSegmentStore),
) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	testData := ldtestdata.DataSource()
	bsStore := &mocks.MockBigSegmentStore{}
	bsStore.TestSetMetadataToCurrentTime()

	addBigSegmentAndFlag(testData)

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = testData
		c.BigSegments = ldcomponents.BigSegments(
			mocks.SingleComponentConfigurer[subsystems.BigSegmentStore]{Instance: bsStore},
		)
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	action(client, bsStore)
}

func TestEvalWithBigSegments(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		doBigSegmentsTest(t, func(client *LDClient, bsStore *mocks.MockBigSegmentStore) {
			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.BigSegmentsHealthy, detail.Reason.GetBigSegmentsStatus())
		})
	})

	t.Run("user found", func(t *testing.T) {
		doBigSegmentsTest(t, func(client *LDClient, bsStore *mocks.MockBigSegmentStore) {
			membership := ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(
				[]string{makeBigSegmentRef(bigSegmentKey, 1)}, nil)
			bsStore.TestSetMembership(bigsegments.HashForContextKey(evalTestUser.Key()), membership)

			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.True(t, value)
			assert.Equal(t, ldreason.BigSegmentsHealthy, detail.Reason.GetBigSegmentsStatus())
		})
	})

	t.Run("store error", func(t *testing.T) {
		doBigSegmentsTest(t, func(client *LDClient, bsStore *mocks.MockBigSegmentStore) {
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
