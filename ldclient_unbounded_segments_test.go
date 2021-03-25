package ldclient

import (
	"errors"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/unboundedsegments"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers/ldtestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unboundedSegmentKey = "segmentkey"

func addUnboundedSegmentAndFlag(testData *ldtestdata.TestDataSource) {
	segment := ldbuilders.NewSegmentBuilder(unboundedSegmentKey).
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

func doUnboundedSegmentsTest(
	t *testing.T,
	action func(client *LDClient, ubsStore *sharedtest.MockUnboundedSegmentStore),
) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	testData := ldtestdata.DataSource()
	ubsStore := &sharedtest.MockUnboundedSegmentStore{}
	ubsStore.TestSetMetadataToCurrentTime()

	addUnboundedSegmentAndFlag(testData)

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = testData
		c.UnboundedSegments = ldcomponents.UnboundedSegments(
			sharedtest.SingleUnboundedSegmentStoreFactory{Store: ubsStore},
		)
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	action(client, ubsStore)
}

func TestEvalWithUnboundedSegments(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		doUnboundedSegmentsTest(t, func(client *LDClient, ubsStore *sharedtest.MockUnboundedSegmentStore) {
			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.UnboundedSegmentsHealthy, detail.Reason.GetUnboundedSegmentsStatus())
		})
	})

	t.Run("user found", func(t *testing.T) {
		doUnboundedSegmentsTest(t, func(client *LDClient, ubsStore *sharedtest.MockUnboundedSegmentStore) {
			ubsStore.TestSetMembership(unboundedsegments.HashForUserKey(evalTestUser.GetKey()),
				ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{unboundedSegmentKey + ":1"}, nil))

			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.True(t, value)
			assert.Equal(t, ldreason.UnboundedSegmentsHealthy, detail.Reason.GetUnboundedSegmentsStatus())
		})
	})

	t.Run("store error", func(t *testing.T) {
		doUnboundedSegmentsTest(t, func(client *LDClient, ubsStore *sharedtest.MockUnboundedSegmentStore) {
			ubsStore.TestSetMembershipError(errors.New("sorry"))

			value, detail, err := client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.UnboundedSegmentsStoreError, detail.Reason.GetUnboundedSegmentsStatus())
		})
	})

	t.Run("store not configured", func(t *testing.T) {
		// deliberately not using a configuration with an unbounded segment store here
		withClientEvalTestParams(func(p clientEvalTestParams) {
			addUnboundedSegmentAndFlag(p.data)

			value, detail, err := p.client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
			require.NoError(t, err)
			assert.False(t, value)
			assert.Equal(t, ldreason.UnboundedSegmentsNotConfigured, detail.Reason.GetUnboundedSegmentsStatus())
		})
	})
}
