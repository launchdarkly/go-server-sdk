package fdv2proto

// Selector represents a particular snapshot of data.
type Selector struct {
	state   string
	version int
}

// NoSelector returns a nil Selector, representing the lack of one. It is
// here only for readability at call sites.
func NoSelector() *Selector {
	return nil
}

// NewSelector creates a new Selector from a state string and version.
func NewSelector(state string, version int) *Selector {
	return &Selector{state: state, version: version}
}

// IsSet returns true if the Selector is not nil.
func (s *Selector) IsSet() bool {
	return s != nil
}

// State returns the state string of the Selector. This cannot be called if the Selector is nil.
func (s *Selector) State() string {
	return s.state
}

// Version returns the version of the Selector. This cannot be called if the Selector is nil.
func (s *Selector) Version() int {
	return s.version
}
