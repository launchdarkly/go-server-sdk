package interfaces

// DataSource describes the interface for an object that receives feature flag data.
type DataSource interface {
	Initialized() bool
	Close() error
	Start(closeWhenReady chan<- struct{})
}
