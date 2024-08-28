package subsystems

type Initializer interface {
	Fetch() error
}

type Synchronizer interface {
	Start()
	Stop()
}

type SynchronizersConfiguration struct {
	Primary   Synchronizer
	Secondary Synchronizer
}

type DataSystemConfiguration struct {
	Store DataStore
	// Initializers obtain data for the SDK in a one-shot manner at startup. Their job is to get the SDK
	// into a state where it is serving somewhat fresh values as fast as possible.
	Initializers  []Initializer
	Synchronizers SynchronizersConfiguration
}

/**

DataSystemConfiguration {
   Store: ldcomponents.Empty(), || ldcomponents.PersistentStore(
   Initializers: []ldcomponents.Initializer{
		ldcomponents.PollFDv2()
   },
   Synchronizers: ldcomponents.SynchronizersConfiguration{
		Primary: ldcomponents.StreamingFDv2(),
		Secondary: ldcomponents.PollFDv2()
	}
}
*/
