package fdv2proto

import (
	"encoding/json"
	"errors"
)

type ChangeType string

const (
	ChangeTypePut    = ChangeType("put")
	ChangeTypeDelete = ChangeType("delete")
)

type Change struct {
	Action  ChangeType
	Kind    ObjectKind
	Key     string
	Version int
	Object  json.RawMessage
}

type ChangeSet struct {
	intentCode IntentCode
	changes  []Change
	selector Selector
}

func (c *ChangeSet) IntentCode() IntentCode{
	return c.intentCode
}

func (c *ChangeSet) Changes() []Change {
	return c.changes
}

func (c *ChangeSet) Selector() Selector {
	return c.selector
}

type ChangeSetBuilder struct {
	intent  *ServerIntent
	changes []Change
}

func NewChangeSetBuilder() *ChangeSetBuilder {
	return &ChangeSetBuilder{}
}

func (c *ChangeSetBuilder) NoChanges() *ChangeSet {
	return &ChangeSet{
		intentCode: IntentNone,
		selector: NoSelector(),
		changes:  nil,
	}
}

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
		selector: selector,
		changes:  c.changes,
	}
	c.changes = nil
	c.intent.Payload.Code =
	return changes, nil
}

func (c *ChangeSetBuilder) AddPut(kind ObjectKind, key string, version int, object json.RawMessage) {
	c.changes = append(c.changes, Change{
		Action:  ChangeTypePut,
		Kind:    kind,
		Key:     key,
		Version: version,
		Object:  object,
	})
}

func (c *ChangeSetBuilder) AddDelete(kind ObjectKind, key string, version int) {
	c.changes = append(c.changes, Change{
		Action:  ChangeTypeDelete,
		Kind:    kind,
		Key:     key,
		Version: version,
	})
}
