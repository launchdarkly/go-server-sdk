package ldclient

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

const testSdkKey = "test-sdk-key"

var testUser = lduser.NewUser("test-user-key")

var alwaysTrueFlag = ldbuilders.NewFlagBuilder("always-true-flag").SingleVariation(ldvalue.Bool(true)).Build()

func basicClientContext() interfaces.ClientContext {
	return newClientContextImpl(testSdkKey, Config{Loggers: ldlog.NewDisabledLoggers()}, nil, nil)
}

func makeInMemoryDataStore() interfaces.DataStore {
	store, _ := ldcomponents.InMemoryDataStore().CreateDataStore(basicClientContext())
	return store
}

type singleDataStoreFactory struct {
	dataStore interfaces.DataStore
}

func (f singleDataStoreFactory) CreateDataStore(context interfaces.ClientContext) (interfaces.DataStore, error) {
	return f.dataStore, nil
}

type singleDataSourceFactory struct {
	dataSource interfaces.DataSource
}

func (f singleDataSourceFactory) CreateDataSource(context interfaces.ClientContext, store interfaces.DataStore) (interfaces.DataSource, error) {
	return f.dataSource, nil
}

type singleEventProcessorFactory struct {
	eventProcessor ldevents.EventProcessor
}

func (f singleEventProcessorFactory) CreateEventProcessor(context interfaces.ClientContext) (ldevents.EventProcessor, error) {
	return f.eventProcessor, nil
}

type mockDataSource struct {
	IsInitialized bool
	CloseFn       func() error
	StartFn       func(chan<- struct{})
}

func (u mockDataSource) Initialized() bool {
	return u.IsInitialized
}

func (u mockDataSource) Close() error {
	if u.CloseFn == nil {
		return nil
	}
	return u.CloseFn()
}

func (u mockDataSource) Start(closeWhenReady chan<- struct{}) {
	if u.StartFn == nil {
		return
	}
	u.StartFn(closeWhenReady)
}

type testEventProcessor struct {
	events []ldevents.Event
}

func (t *testEventProcessor) SendEvent(e ldevents.Event) {
	t.events = append(t.events, e)
}

func (t *testEventProcessor) Flush() {}

func (t *testEventProcessor) Close() error {
	return nil
}
