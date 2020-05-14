package interfaces

// StoreCollection is used by the PersistentDataStore interface.
type StoreCollection struct {
	Kind  VersionedDataKind
	Items []VersionedData
}
