package datastatus

type DataStatus string

const (
	// Unknown means there is no known status.
	Unknown = DataStatus("unknown")
	// Authoritative means the data is from an authoritative source. Authoritative data may be replicated
	// from the SDK into any connected persistent store (in write mode), and causes the SDK to transition from
	// the Defaults/Cached states to Refreshed.
	Authoritative = DataStatus("authoritative")
	// Derivative means the data may be stale, such as from a local file or persistent store. Derivative data
	// is not replicated to any connected persistent store, and causes the SDK to transition from the Defaults
	// state to Cached only.
	Derivative = DataStatus("derivative")
)
