package datasystem

type DataStatus string

const (
	// Defaults means the SDK has no data and will evaluate flags using the application-provided default values.
	Defaults = DataStatus("defaults")
	// Cached means the SDK has data, not necessarily the latest, which will be used to evaluate flags.
	Cached = DataStatus("cached")
	// Refreshed means the SDK has obtained, at least once, the latest known data from LaunchDarkly.
	Refreshed = DataStatus("refreshed")
)
