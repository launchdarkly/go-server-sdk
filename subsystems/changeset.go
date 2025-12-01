package subsystems

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// ChangeType specifies if an object is being upserted or deleted.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type ChangeType string

const (
	// ChangeTypePut represents an object being upserted.
	ChangeTypePut = ChangeType("put")

	// ChangeTypeDelete represents an object being deleted.
	ChangeTypeDelete = ChangeType("delete")
)

// Change represents a change to a piece of data, such as an update or deletion.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type Change struct {
	Action  ChangeType
	Kind    ObjectKind
	Key     string
	Version int
	Object  json.RawMessage
}

// ChangeSet represents a list of changes to be applied.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type ChangeSet struct {
	intentCode IntentCode
	changes    []Change
	selector   Selector

	mu         *sync.Mutex
	collection []ldstoretypes.Collection
}

// IntentCode represents the intent of the changeset.
func (c *ChangeSet) IntentCode() IntentCode {
	return c.intentCode
}

// Changes returns the individual changes that should be applied according
// to the intent.
func (c *ChangeSet) Changes() []Change {
	return c.changes
}

// Selector identifies the version of the changes.
func (c *ChangeSet) Selector() Selector {
	return c.selector
}

// Collections converts the changeset into a list of collections suitable for
// insertion into a data store, relaying to FDv1 clients, etc.
func (c *ChangeSet) Collections() ([]ldstoretypes.Collection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.collection != nil {
		return c.collection, nil
	}

	if c.changes == nil {
		c.collection = []ldstoretypes.Collection{}
		return c.collection, nil
	}

	collection, err := toStorableItems(c.changes)
	if err != nil {
		return nil, err
	}

	c.collection = collection
	return c.collection, nil
}

// toStorableItems converts a list of FDv2 events to a list of collections suitable for insertion
// into a data store.
func toStorableItems(deltas []Change) ([]ldstoretypes.Collection, error) {
	collections := make(kindMap)
	for _, event := range deltas {
		kind, ok := event.Kind.ToFDV1()
		if !ok {
			// If we don't recognize this kind, it's not an error and should be ignored for forwards
			// compatibility.
			continue
		}

		switch event.Action {
		case ChangeTypePut:
			// A put requires deserializing the item. We delegate to the optimized streaming JSON
			// parser.
			reader := jreader.NewReader(event.Object)
			item, err := kind.DeserializeFromJSONReader(&reader)
			if err != nil {
				return nil, err
			}
			collections[kind] = append(collections[kind], ldstoretypes.KeyedItemDescriptor{
				Key:  event.Key,
				Item: item,
			})
		case ChangeTypeDelete:
			// A deletion is represented by a tombstone, which is an ItemDescriptor with a version and nil item.
			collections[kind] = append(collections[kind], ldstoretypes.KeyedItemDescriptor{
				Key:  event.Key,
				Item: ldstoretypes.ItemDescriptor{Version: event.Version, Item: nil},
			})
		default:
			// An unknown action isn't an error, and should be ignored for forwards compatibility.
			continue
		}
	}

	return collections.flatten(), nil
}

// ChangeSetBuilder is a helper for constructing a ChangeSet.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type ChangeSetBuilder struct {
	intent  *ServerIntent
	changes []Change
}

// NewChangeSetBuilder creates a new ChangeSetBuilder, which is empty by default.
//
// This function is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
func NewChangeSetBuilder() *ChangeSetBuilder {
	return &ChangeSetBuilder{}
}

// NoChanges represents an intent that the current data is up-to-date and doesn't
// require changes.
func (c *ChangeSetBuilder) NoChanges() *ChangeSet {
	return &ChangeSet{
		intentCode: IntentNone,
		selector:   NoSelector(),
		changes:    nil,
		mu:         &sync.Mutex{},
	}
}

// Empty returns an empty ChangeSet, which is useful for initializing a client
// without data, or for clearing out all existing data.
func (c *ChangeSetBuilder) Empty(selector Selector) *ChangeSet {
	return &ChangeSet{
		intentCode: IntentTransferFull,
		selector:   selector,
		changes:    nil,
		mu:         &sync.Mutex{},
	}
}

// Start begins a new change set with a given intent.
func (c *ChangeSetBuilder) Start(intent ServerIntent) *ChangeSetBuilder {
	c.intent = &intent
	c.changes = nil
	return c
}

// ExpectChanges ensures that the current ChangeSetBuilder is prepared to handle changes.
//
// If a data source's initial connection reflects an update-to-date status, we
// need to keep the provided server-intent. This allows subsequent changes to
// come down the line without an explicit server-intent.
//
// However, to maintain logical consistency, we need to ensure that the intent
// is set to IntentTransferChanges.
func (c *ChangeSetBuilder) ExpectChanges() error {
	if c.intent == nil {
		return errors.New("changeset: cannot expect changes without a server-intent")
	}

	if c.intent.Payload.Code != IntentNone {
		return nil
	}

	c.intent.Payload.Code = IntentTransferChanges
	return nil
}

// Reset clears any existing changes, while preserving the current intent.
func (c *ChangeSetBuilder) Reset() {
	c.changes = nil
}

// Finish identifies a changeset with a selector, and returns the completed changeset.
// It clears any existing changes, while preserving the current intent, so that the builder can be reused.
func (c *ChangeSetBuilder) Finish(selector Selector) (*ChangeSet, error) {
	if c.intent == nil {
		return nil, errors.New("changeset: cannot complete without a server-intent")
	}
	changes := &ChangeSet{
		intentCode: c.intent.Payload.Code,
		selector:   selector,
		changes:    c.changes,
		mu:         &sync.Mutex{},
	}
	c.changes = nil

	// Once a full transfer has been processed, all future changes should be
	// assumed to be changes. Flag delivery can override this behavior by
	// sending a new ServerIntent to any connected stream.
	if c.intent.Payload.Code == IntentTransferFull {
		c.intent.Payload.Code = IntentTransferChanges
	}
	return changes, nil
}

// AddPut adds a new object to the changeset.
func (c *ChangeSetBuilder) AddPut(kind ObjectKind, key string, version int, object json.RawMessage) *ChangeSetBuilder {
	c.changes = append(c.changes, Change{
		Action:  ChangeTypePut,
		Kind:    kind,
		Key:     key,
		Version: version,
		Object:  object,
	})

	return c
}

// AddDelete adds a deletion to the changeset.
func (c *ChangeSetBuilder) AddDelete(kind ObjectKind, key string, version int) *ChangeSetBuilder {
	c.changes = append(c.changes, Change{
		Action:  ChangeTypeDelete,
		Kind:    kind,
		Key:     key,
		Version: version,
	})

	return c
}
