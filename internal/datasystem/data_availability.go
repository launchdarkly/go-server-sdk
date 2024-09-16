package datasystem

type DataAvailability string

const (
	// Defaults means the SDK has no data and will evaluate flags using the application-provided default values.
	Defaults = DataAvailability("defaults")
	// Cached means the SDK has data, not necessarily the latest, which will be used to evaluate flags.
	Cached = DataAvailability("cached")
	// Refreshed means the SDK has obtained, at least once, the latest known data from LaunchDarkly.
	Refreshed = DataAvailability("refreshed")
)
