package ldtestdatav2

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v4/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var threeStringValues = []ldvalue.Value{ldvalue.String("red"), ldvalue.String("green"), ldvalue.String("blue")}

type testDataSourceTestParams struct {
	td      *TestDataSynchronizer
	updates *mocks.MockDataSourceUpdates
}

func testDataSourceTest(t *testing.T, action func(testDataSourceTestParams)) {
	t.Helper()
	var p testDataSourceTestParams
	p.td = DataSource()
	action(p)
}

func (p testDataSourceTestParams) withDataSynchronizer(t *testing.T, selector subsystems.DataSelector, action func(subsystems.DataSynchronizer, <-chan subsystems.DataSynchronizerResult)) {
	t.Helper()
	context := subsystems.BasicClientContext{DataSourceUpdateSink: p.updates}
	synchronizer, err := p.td.Build(context)
	require.NoError(t, err)
	defer synchronizer.Close()

	resultChan := synchronizer.Sync(selector)

	action(synchronizer, resultChan)
}

func TestCanBeUsedAsInitializer(t *testing.T) {
	td := DataSource()
	td.Update(td.Flag("flag1").On(true))
	td.Update(td.Flag("flag2").On(false))

	initializer, err := td.AsInitializer().Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	basis, _, err := initializer.Fetch(selector, context.Background())
	assert.NoError(t, err)

	changes := basis.ChangeSet.Changes()
	assert.Len(t, changes, 2)

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Key < changes[j].Key
	})
	assert.Equal(t, subsystems.IntentTransferFull, basis.ChangeSet.IntentCode())
	assert.Equal(t, "flag1", changes[0].Key)
	assert.Equal(t, 1, changes[0].Version)
	assert.Equal(t, "flag2", changes[1].Key)
	assert.Equal(t, 1, changes[1].Version)

	td.Update(td.Flag("flag1").On(false))

	basis, _, err = initializer.Fetch(selector, context.Background())
	assert.NoError(t, err)

	changes = basis.ChangeSet.Changes()
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Key < changes[j].Key
	})
	assert.Len(t, changes, 2)
	assert.Equal(t, subsystems.IntentTransferFull, basis.ChangeSet.IntentCode())
	assert.Equal(t, "flag1", changes[0].Key)
	assert.Equal(t, 2, changes[0].Version)
	assert.Equal(t, "flag2", changes[1].Key)
	assert.Equal(t, 1, changes[1].Version)
}

func TestInitializesWithEmptyData(t *testing.T) {
	td := DataSource()
	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)

	result := <-resultChan
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 0)
	assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
}

func TestInitializesWithFlags(t *testing.T) {
	td := DataSource()
	td.Update(td.Flag("flag1").On(true))
	td.Update(td.Flag("flag2").On(false))

	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)

	result := <-resultChan
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 2)
	assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
}

func TestUpdatesFlags(t *testing.T) {
	td := DataSource()
	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)

	// Throw away the initial result
	result := <-resultChan

	td.Update(td.Flag("flag1").On(true))

	result = <-resultChan

	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferChanges, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
}

func TestSupportsMultipleSynchronizers(t *testing.T) {
	td := DataSource()
	sync1, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	sync2, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan1 := sync1.Sync(selector)
	resultChan2 := sync2.Sync(selector)

	// Throw away the initial result
	result := <-resultChan1
	result = <-resultChan2

	td.Update(td.Flag("flag1").On(true))

	result = <-resultChan1
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferChanges, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)

	result = <-resultChan2
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferChanges, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)

	sync2.Close()

	th.AssertChannelClosed(t, resultChan2, time.Millisecond, "result channel should be closed")

	td.Update(td.Flag("flag1").On(false))

	result = <-resultChan1
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferChanges, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
}

func TestUpdatesStatus(t *testing.T) {
	td := DataSource()
	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)

	// Throw away the initial result
	result := <-resultChan

	td.UpdateStatus(interfaces.DataSourceStateInterrupted, interfaces.DataSourceErrorInfo{Kind: interfaces.DataSourceErrorKindNetworkError})

	result = <-resultChan

	assert.Nil(t, result.ChangeSet)
	assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)
	assert.Equal(t, interfaces.DataSourceErrorInfo{Kind: interfaces.DataSourceErrorKindNetworkError}, result.Error)
}

func TestAddsOrUpdatesPreconfiguredFlag(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).On(true).Build()

	td := DataSource()
	td.UsePreconfiguredFlag(flag)

	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)

	result := <-resultChan
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)

	assert.Equal(t, flag.Key, result.ChangeSet.Changes()[0].Key)
	assert.Equal(t, flag.Version, result.ChangeSet.Changes()[0].Version)

	flag = ldbuilders.NewFlagBuilder("flagkey").Version(2).On(true).Build()
	td.UsePreconfiguredFlag(flag)

	result = <-resultChan
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferChanges, result.ChangeSet.IntentCode())
	assert.Equal(t, flag.Key, result.ChangeSet.Changes()[0].Key)
	assert.Equal(t, flag.Version, result.ChangeSet.Changes()[0].Version)
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
}

func TestAddsOrUpdatesPreconfiguredSegment(t *testing.T) {
	segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Included("a").Build()

	td := DataSource()
	td.UsePreconfiguredSegment(segment)

	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)

	result := <-resultChan
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)

	assert.Equal(t, segment.Key, result.ChangeSet.Changes()[0].Key)
	assert.Equal(t, segment.Version, result.ChangeSet.Changes()[0].Version)

	segment = ldbuilders.NewSegmentBuilder("segmentkey").Version(2).Included("b").Build()
	td.UsePreconfiguredSegment(segment)

	result = <-resultChan
	assert.NotNil(t, result.ChangeSet)
	assert.Len(t, result.ChangeSet.Changes(), 1)
	assert.Equal(t, subsystems.IntentTransferChanges, result.ChangeSet.IntentCode())
	assert.Equal(t, segment.Key, result.ChangeSet.Changes()[0].Key)
	assert.Equal(t, segment.Version, result.ChangeSet.Changes()[0].Version)
	assert.Equal(t, interfaces.DataSourceStateValid, result.State)
}

func TestClosingSynchronizerClosesResultChannel(t *testing.T) {
	td := DataSource()
	sync, err := td.Build(subsystems.BasicClientContext{})
	assert.NoError(t, err)

	selector := mocks.NewMockDataSelector(subsystems.NoSelector())
	resultChan := sync.Sync(selector)
	<-resultChan // throw away the initial result

	sync.Close()

	th.AssertChannelClosed(t, resultChan, time.Millisecond, "result channel should be closed")
}
