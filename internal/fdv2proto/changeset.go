package fdv2proto

import (
	"encoding/json"
	"errors"
)

// ChangeType specifies if an object is being upserted or deleted.
type ChangeType string

const (
	// ChangeTypePut represents an object being upserted.
	ChangeTypePut = ChangeType("put")

	// ChangeTypeDelete represents an object being deleted.
	ChangeTypeDelete = ChangeType("delete")
)

// Change represents a change to a piece of data, such as an update or deletion.
type Change struct {
	Action  ChangeType
	Kind    ObjectKind
	Key     string
	Version int
	Object  json.RawMessage
}

// ChangeSet represents a list of changes to be applied.
type ChangeSet struct {
	intentCode IntentCode
	changes    []Change
	selector   Selector
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

// ChangeSetBuilder is a helper for constructing a ChangeSet.
type ChangeSetBuilder struct {
	intent  *ServerIntent
	changes []Change
}

// NewChangeSetBuilder creates a new ChangeSetBuilder, which is empty by default.
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
	}
}

// Start begins a new change set with a given intent.
func (c *ChangeSetBuilder) Start(intent ServerIntent) error {
	c.intent = &intent
	c.changes = nil
	return nil
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
	}
	c.changes = nil
	if c.intent.Payload.Code == IntentTransferFull {
		//nolint:godox
		// TODO(SDK-931): We have an awkward situation where we don't get a new intent after receiving a payload
		// transferred message, so we need to assume the new intent. But we don't get new Reason/ID/Target, so we don't
		// have complete information.
		c.intent.Payload.Code = IntentTransferChanges
	}
	return changes, nil
}

// AddPut adds a new object to the changeset.
func (c *ChangeSetBuilder) AddPut(kind ObjectKind, key string, version int, object json.RawMessage) {
	c.changes = append(c.changes, Change{
		Action:  ChangeTypePut,
		Kind:    kind,
		Key:     key,
		Version: version,
		Object:  object,
	})
}

// AddDelete adds a deletion to the changeset.
func (c *ChangeSetBuilder) AddDelete(kind ObjectKind, key string, version int) {
	c.changes = append(c.changes, Change{
		Action:  ChangeTypeDelete,
		Kind:    kind,
		Key:     key,
		Version: version,
	})
}
