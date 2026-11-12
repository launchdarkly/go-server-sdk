package datasourcev2

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFDv1PollingProcessorInitializerReturnsDataWithNoSelector(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservices.NewServerSDKData().Flags(alwaysTrueFlag)

	handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewFDv1PollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		basis, _, err := processor.Fetch(ds, context.Background())
		require.NoError(t, err)
		assert.Equal(t, subsystems.IntentTransferFull, basis.ChangeSet.IntentCode())
		assert.Len(t, basis.ChangeSet.Changes(), 1)
		assert.False(t, basis.ChangeSet.Selector().IsDefined())
	})
}

func TestFDv1PollingProcessorSynchronizerReturnsDataWithNoSelector(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservices.NewServerSDKData().Flags(alwaysTrueFlag)

	handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewFDv1PollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		result := <-processor.Sync(ds)
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		require.NotNil(t, result.ChangeSet)
		assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
		assert.False(t, result.ChangeSet.Selector().IsDefined())
	})
}
