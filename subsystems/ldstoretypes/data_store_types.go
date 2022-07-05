package ldstoretypes

// DataKind represents a separately namespaced collection of storable data items.
//
// The SDK passes instances of this type to the data store to specify whether it is referring to
// a feature flag, a user segment, etc. The data store implementation should not look for a
// specific data kind (such as feature flags), but should treat all data kinds generically.
//
// The SDK's standard implementations of this type are available in the
// ldcomponents/ldstoreimpl package.
type DataKind interface {
	GetName() string
	Serialize(item ItemDescriptor) []byte
	Deserialize(data []byte) (ItemDescriptor, error)
}

// ItemDescriptor is a versioned item (or placeholder) storable in a DataStore.
//
// This is used for data stores that directly store objects as-is, as the default in-memory
// store does. Items are typed as interface{}; the store should not know or care what the
// actual object is.
//
// For any given key within a DataKind, there can be either an existing item with a
// version, or a "tombstone" placeholder representing a deleted item (also with a version).
// Deleted item placeholders are used so that if an item is first updated with version N and
// then deleted with version N+1, but the SDK receives those changes out of order, version N
// will not overwrite the deletion.
//
// Persistent data stores use SerializedItemDescriptor instead.
type ItemDescriptor struct {
	// Version is the version number of this data, provided by the SDK.
	Version int
	// Item is the data item, or nil if this is a placeholder for a deleted item.
	Item interface{}
}

// NotFound is a convenience method to return a value indicating no such item exists.
func (s ItemDescriptor) NotFound() ItemDescriptor {
	return ItemDescriptor{Version: -1, Item: nil}
}

// SerializedItemDescriptor is a versioned item (or placeholder) storable in a PersistentDataStore.
//
// This is equivalent to ItemDescriptor, but is used for persistent data stores. The
// SDK will convert each data item to and from its serialized string form; the persistent data
// store deals only with the serialized form.
type SerializedItemDescriptor struct {
	// Version is the version number of this data, provided by the SDK.
	Version int
	// Deleted is true if this is a placeholder (tombstone) for a deleted item. If so,
	// SerializedItem will still contain a byte string representing the deleted item, but
	// the persistent store implementation has the option of not storing it if it can represent the
	// placeholder in a more efficient way.
	Deleted bool
	// SerializedItem is the data item's serialized representation. For a deleted item placeholder,
	// instead of passing nil, the SDK will provide a special value that can be stored if necessary
	// (see Deleted).
	SerializedItem []byte
}

// NotFound is a convenience method to return a value indicating no such item exists.
func (s SerializedItemDescriptor) NotFound() SerializedItemDescriptor {
	return SerializedItemDescriptor{Version: -1, SerializedItem: nil}
}

// KeyedItemDescriptor is a key-value pair containing a ItemDescriptor.
type KeyedItemDescriptor struct {
	// Key is the unique key of this item within its DataKind.
	Key string
	// Item is the versioned item.
	Item ItemDescriptor
}

// KeyedSerializedItemDescriptor is a key-value pair containing a SerializedItemDescriptor.
type KeyedSerializedItemDescriptor struct {
	// Key is the unique key of this item within its DataKind.
	Key string
	// Item is the versioned serialized item.
	Item SerializedItemDescriptor
}

// Collection is a list of data store items for a DataKind.
type Collection struct {
	Kind  DataKind
	Items []KeyedItemDescriptor
}

// SerializedCollection is a list of serialized data store items for a DataKind.
type SerializedCollection struct {
	Kind  DataKind
	Items []KeyedSerializedItemDescriptor
}
