package ldtestdata

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"golang.org/x/exp/slices"
)

// TestDataSource is a test fixture that provides dynamically updatable feature flag state in a
// simplified form to an SDK client in test scenarios.
//
// See package description for more details and usage examples.
type TestDataSource struct {
	currentFlags    map[string]ldstoretypes.ItemDescriptor
	currentBuilders map[string]*FlagBuilder
	currentSegments map[string]ldstoretypes.ItemDescriptor
	instances       []*testDataSourceImpl
	lock            sync.Mutex
}

type testDataSourceImpl struct {
	owner   *TestDataSource
	updates subsystems.DataSourceUpdateSink
}

// DataSource creates an instance of [TestDataSource].
//
// Storing this object in the DataSource field of [github.com/launchdarkly/go-server-sdk/v7.Config]
// causes the SDK client to use the test data. Any subsequent changes made using methods like
// [TestDataSource.Update] will propagate to all LDClient instances that are using this data source.
func DataSource() *TestDataSource {
	return &TestDataSource{
		currentFlags:    make(map[string]ldstoretypes.ItemDescriptor),
		currentBuilders: make(map[string]*FlagBuilder),
		currentSegments: make(map[string]ldstoretypes.ItemDescriptor),
	}
}

// Flag creates or copies a [FlagBuilder] for building a test flag configuration.
//
// If this flag key has already been defined in this TestDataSource instance, then the builder
// starts with the same configuration that was last provided for this flag.
//
// Otherwise, it starts with a new default configuration in which the flag has true and false
// variations, is true for all users when targeting is turned on and false otherwise, and
// currently has targeting turned on. You can change any of those properties, and provide more
// complex behavior, using the FlagBuilder methods.
//
// Once you have set the desired configuration, pass the builder to Update.
func (t *TestDataSource) Flag(key string) *FlagBuilder {
	t.lock.Lock()
	defer t.lock.Unlock()
	existingBuilder := t.currentBuilders[key]
	if existingBuilder == nil {
		return newFlagBuilder(key).BooleanFlag()
	}
	return copyFlagBuilder(existingBuilder)
}

// Update updates the test data with the specified flag configuration.
//
// This has the same effect as if a flag were added or modified on the LaunchDarkly dashboard.
// It immediately propagates the flag change to any LDClient instance(s) that you have already
// configured to use this TestDataSource. If no LDClient has been started yet, it simply adds
// this flag to the test data which will be provided to any LDClient that you subsequently
// configure.
//
// Any subsequent changes to this FlagBuilder instance do not affect the test data, unless
// you call Update again.
func (t *TestDataSource) Update(flagBuilder *FlagBuilder) *TestDataSource {
	key := flagBuilder.key
	clonedBuilder := copyFlagBuilder(flagBuilder)
	t.updateInternal(key, flagBuilder.createFlag, clonedBuilder)
	return t
}

// UpdateStatus simulates a change in the data source status.
//
// Use this if you want to test the behavior of application code that uses
// LDClient.GetDataSourceStatusProvider to track whether the data source is having problems (for example,
// a network failure interruptsingthe streaming connection). It does not actually stop the
// TestDataSource from working, so even if you have simulated an outage, calling Update will still send
// updates.
func (t *TestDataSource) UpdateStatus(
	newState interfaces.DataSourceState,
	newError interfaces.DataSourceErrorInfo,
) *TestDataSource {
	t.lock.Lock()
	instances := slices.Clone(t.instances)
	t.lock.Unlock()

	for _, instance := range instances {
		instance.updates.UpdateStatus(newState, newError)
	}

	return t
}

// UsePreconfiguredFlag copies a full feature flag data model object into the test data.
//
// It immediately propagates the flag change to any LDClient instance(s) that you have already
// configured to use this TestDataSource. If no LDClient has been started yet, it simply adds
// this flag to the test data which will be provided to any LDClient that you subsequently
// configure.
//
// Use this method if you need to use advanced flag configuration properties that are not supported by
// the simplified FlagBuilder API. Otherwise it is recommended to use the regular Flag/Update
// mechanism to avoid dependencies on details of the data model.
//
// You cannot make incremental changes with Flag/Update to a flag that has been added in this way;
// you can only replace it with an entirely new flag configuration.
//
// To construct an instance of ldmodel.FeatureFlag, rather than accessing the fields directly it is
// recommended to use the builder API in [github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders].
func (t *TestDataSource) UsePreconfiguredFlag(flag ldmodel.FeatureFlag) *TestDataSource {
	t.updateInternal(
		flag.Key,
		func(version int) ldmodel.FeatureFlag {
			f := flag
			if f.Version < version {
				f.Version = version
			}
			return f
		},
		nil,
	)
	return t
}

// UsePreconfiguredSegment copies a full user segment data model object into the test data.
//
// It immediately propagates the flag change to any LDClient instance(s) that you have already
// configured to use this TestDataSource. If no LDClient has been started yet, it simply adds
// this flag to the test data which will be provided to any LDClient that you subsequently
// configure.
//
// This method is currently the only way to inject user segment data, since there is no builder
// API for segments. It is mainly intended for the SDK's own tests of user segment functionality,
// since application tests that need to produce a desired evaluation state could do so more easily
// by just setting flag values.
//
// To construct an instance of ldmodel.Segment, rather than accessing the fields directly it is
// recommended to use the builder API in [github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders].
func (t *TestDataSource) UsePreconfiguredSegment(segment ldmodel.Segment) *TestDataSource {
	t.lock.Lock()
	oldItem := t.currentSegments[segment.Key]
	newSegment := segment
	newSegment.Version = oldItem.Version + 1
	newItem := ldstoretypes.ItemDescriptor{Version: newSegment.Version, Item: &newSegment}
	t.currentSegments[segment.Key] = newItem
	instances := slices.Clone(t.instances)
	t.lock.Unlock()

	for _, instance := range instances {
		instance.updates.Upsert(ldstoreimpl.Segments(), segment.Key, newItem)
	}

	return t
}

func (t *TestDataSource) updateInternal(
	key string,
	makeFlag func(int) ldmodel.FeatureFlag,
	builder *FlagBuilder,
) {
	t.lock.Lock()
	oldItem := t.currentFlags[key]
	newVersion := oldItem.Version + 1
	newFlag := makeFlag(newVersion)
	newItem := ldstoretypes.ItemDescriptor{Version: newVersion, Item: &newFlag}
	t.currentFlags[key] = newItem
	t.currentBuilders[key] = builder
	instances := slices.Clone(t.instances)
	t.lock.Unlock()

	for _, instance := range instances {
		instance.updates.Upsert(ldstoreimpl.Features(), key, newItem)
	}
}

// Build is called internally by the SDK to associate this test data source with an
// LDClient instance. You do not need to call this method.
func (t *TestDataSource) Build(context subsystems.ClientContext) (subsystems.DataSource, error) {
	instance := &testDataSourceImpl{owner: t, updates: context.GetDataSourceUpdateSink()}
	t.lock.Lock()
	t.instances = append(t.instances, instance)
	t.lock.Unlock()
	return instance, nil
}

func (t *TestDataSource) makeInitData() []ldstoretypes.Collection {
	t.lock.Lock()
	defer t.lock.Unlock()
	flags := make([]ldstoretypes.KeyedItemDescriptor, 0, len(t.currentFlags))
	segments := make([]ldstoretypes.KeyedItemDescriptor, 0, len(t.currentSegments))
	for key, item := range t.currentFlags {
		flags = append(flags, ldstoretypes.KeyedItemDescriptor{Key: key, Item: item})
	}
	for key, item := range t.currentSegments {
		segments = append(segments, ldstoretypes.KeyedItemDescriptor{Key: key, Item: item})
	}
	return []ldstoretypes.Collection{
		{Kind: ldstoreimpl.Features(), Items: flags},
		{Kind: ldstoreimpl.Segments(), Items: segments},
	}
}

func (t *TestDataSource) closedInstance(instance *testDataSourceImpl) {
	t.lock.Lock()
	defer t.lock.Unlock()
	for i, in := range t.instances {
		if in == instance {
			copy(t.instances[i:], t.instances[i+1:])
			t.instances[len(t.instances)-1] = nil
			t.instances = t.instances[:len(t.instances)-1]
			break
		}
	}
}

func (d *testDataSourceImpl) Close() error {
	d.owner.closedInstance(d)
	return nil
}

func (d *testDataSourceImpl) IsInitialized() bool {
	return true
}

func (d *testDataSourceImpl) Start(closeWhenReady chan<- struct{}) {
	_ = d.updates.Init(d.owner.makeInitData())
	d.updates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
	close(closeWhenReady)
}
