package ldtestdatav2

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// TestDataSynchronizer is a test fixture that provides dynamically updatable feature flag state in a
// simplified form to an SDK client in test scenarios.
//
// See package description for more details and usage examples.
type TestDataSynchronizer struct {
	currentFlags         map[string]ldstoretypes.ItemDescriptor
	currentBuilders      map[string]*FlagBuilder
	currentSegments      map[string]ldstoretypes.ItemDescriptor
	changeSetBroadcaster *internal.Broadcaster[subsystems.ChangeSet]
	statusBroadcaster    *internal.Broadcaster[interfaces.DataSynchronizerStatus]
	version              int
	lock                 sync.Mutex
}

// DataSource creates an instance of [TestDataSynchronizer].
//
// Storing this object in the DataSource field of [github.com/launchdarkly/go-server-sdk/v7.Config]
// causes the SDK client to use the test data. Any subsequent changes made using methods like
// [TestDataSynchronizer.Update] will propagate to all LDClient instances that are using this data source.
func DataSource() *TestDataSynchronizer {
	return &TestDataSynchronizer{
		currentFlags:         make(map[string]ldstoretypes.ItemDescriptor),
		currentBuilders:      make(map[string]*FlagBuilder),
		currentSegments:      make(map[string]ldstoretypes.ItemDescriptor),
		changeSetBroadcaster: internal.NewBroadcaster[subsystems.ChangeSet](),
		statusBroadcaster:    internal.NewBroadcaster[interfaces.DataSynchronizerStatus](),
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
func (t *TestDataSynchronizer) Flag(key string) *FlagBuilder {
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
func (t *TestDataSynchronizer) Update(flagBuilder *FlagBuilder) *TestDataSynchronizer {
	key := flagBuilder.key
	clonedBuilder := copyFlagBuilder(flagBuilder)
	t.updateInternal(key, flagBuilder.createFlag, clonedBuilder)
	return t
}

// UpdateStatus simulates a change in the data source status.
//
// Use this if you want to test the behavior of application code that uses
// LDClient.GetDataSourceStatusProvider to track whether the data source is having problems (for example,
// a network failure interrupting the streaming connection). It does not actually stop the
// TestDataSource from working, so even if you have simulated an outage, calling Update will still send
// updates.
func (t *TestDataSynchronizer) UpdateStatus(
	newState interfaces.DataSourceState,
	newError interfaces.DataSourceErrorInfo,
) *TestDataSynchronizer {
	status := interfaces.DataSynchronizerStatus{
		State: newState,
		Error: newError,
	}
	t.statusBroadcaster.Broadcast(status)

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
func (t *TestDataSynchronizer) UsePreconfiguredFlag(flag ldmodel.FeatureFlag) *TestDataSynchronizer {
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
func (t *TestDataSynchronizer) UsePreconfiguredSegment(segment ldmodel.Segment) *TestDataSynchronizer {
	t.lock.Lock()
	oldItem := t.currentSegments[segment.Key]
	newSegment := segment
	newSegment.Version = oldItem.Version + 1
	newItem := ldstoretypes.ItemDescriptor{Version: newSegment.Version, Item: &newSegment}
	t.currentSegments[segment.Key] = newItem
	t.lock.Unlock()

	changeSet, err := t.makeTransferChangesForObject(subsystems.FlagKind, segment.Key, newItem)
	if err != nil {
		return t
	}

	t.changeSetBroadcaster.Broadcast(*changeSet)

	return t
}

func (t *TestDataSynchronizer) updateInternal(
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
	t.lock.Unlock()

	changeSet, err := t.makeTransferChangesForObject(subsystems.FlagKind, key, newItem)
	if err != nil {
		return
	}

	t.changeSetBroadcaster.Broadcast(*changeSet)
}

func (t *TestDataSynchronizer) makeBasis() (*subsystems.Basis, error) {
	changeSet, err := t.makeFullTransferChangeset()
	if err != nil {
		return nil, err
	}

	return &subsystems.Basis{
		ChangeSet: *changeSet,
		Persist:   false,
	}, nil
}

func (t *TestDataSynchronizer) makeFullTransferChangeset() (*subsystems.ChangeSet, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	version := t.version
	t.version++

	builder := subsystems.NewChangeSetBuilder()
	err := builder.Start(subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "",
			Target: version,
			Code:   subsystems.IntentTransferFull,
			Reason: "payload-missing",
		},
	})
	if err != nil {
		return nil, err
	}

	for key, item := range t.currentFlags {
		json, err := json.Marshal(item.Item)
		if err != nil {
			return nil, err
		}
		builder.AddPut(subsystems.FlagKind, key, item.Version, json)
	}

	for key, item := range t.currentSegments {
		json, err := json.Marshal(item.Item)
		if err != nil {
			return nil, err
		}
		builder.AddPut(subsystems.SegmentKind, key, item.Version, json)
	}

	return builder.Finish(subsystems.NewSelector(strconv.Itoa(version), version))
}

func (t *TestDataSynchronizer) makeTransferChangesForObject(
	objectKind subsystems.ObjectKind, key string, item ldstoretypes.ItemDescriptor,
) (*subsystems.ChangeSet, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	version := t.version
	t.version++

	builder := subsystems.NewChangeSetBuilder()
	err := builder.Start(subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "",
			Target: version,
			Code:   subsystems.IntentTransferChanges,
			Reason: "changes",
		},
	})
	if err != nil {
		return nil, err
	}

	json, err := json.Marshal(item.Item)
	if err != nil {
		return nil, err
	}

	builder.AddPut(objectKind, key, item.Version, json)

	return builder.Finish(subsystems.NewSelector(strconv.Itoa(version), version))
}

// Build creates a DataSynchronizer instance that can be used by the SDK client.
func (t *TestDataSynchronizer) Build(clientContext subsystems.ClientContext) (subsystems.DataSynchronizer, error) {
	dataSource := &testDataSourceImpl{
		owner: t,
		quit:  make(chan struct{}),
	}

	return dataSource, nil
}

// AsInitializer returns a ComponentConfigurer that can be used to register this TestDataSynchronizer
// as a data initializer in the SDK's data system.
func (t *TestDataSynchronizer) AsInitializer() subsystems.ComponentConfigurer[subsystems.DataInitializer] {
	return subsystems.AsInitializer(t)
}

type testDataSourceImpl struct {
	owner  *TestDataSynchronizer
	closed internal.AtomicBoolean
	quit   chan struct{}
}

func (d *testDataSourceImpl) Close() error {
	if swapped := d.closed.GetAndSet(true); !swapped {
		close(d.quit)
	}

	return nil
}

func (d *testDataSourceImpl) Name() string {
	return "TestDataSynchronizer"
}

func (d *testDataSourceImpl) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, error) {
	return d.owner.makeBasis()
}

func (d *testDataSourceImpl) Sync(ds subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	resultChan := make(chan subsystems.DataSynchronizerResult)

	changeSetChan := d.owner.changeSetBroadcaster.AddListener()
	statusChan := d.owner.statusBroadcaster.AddListener()

	result := subsystems.DataSynchronizerResult{
		State:        interfaces.DataSourceStateInitializing,
		RevertToFDv1: false,
	}

	go func() {
		if cs, err := d.owner.makeFullTransferChangeset(); err == nil {
			result.ChangeSet = cs
			result.State = interfaces.DataSourceStateValid
		} else {
			result.Error = interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindStoreError,
				StatusCode: 0,
				Message:    err.Error(),
			}
		}

		resultChan <- result
		defer close(resultChan)

		for {
			select {
			case <-d.quit:
				return
			case changeSet, ok := <-changeSetChan:
				if !ok {
					return
				}

				result.ChangeSet = &changeSet
				result.State = interfaces.DataSourceStateValid
				resultChan <- result
			case statusChange, ok := <-statusChan:
				if !ok {
					return
				}

				if statusChange.State != interfaces.DataSourceStateValid {
					result.ChangeSet = nil
				}
				result.State = statusChange.State
				result.Error = statusChange.Error
				resultChan <- result
			}
		}
	}()

	return resultChan
}
