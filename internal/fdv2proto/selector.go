package fdv2proto

type Selector struct {
	state   string
	version int
	set     bool
}

func NoSelector() Selector {
	return Selector{set: false}
}

func NewSelector(state string, version int) Selector {
	return Selector{state: state, version: version, set: true}
}

func (s Selector) IsSet() bool {
	return s.set
}

func (s Selector) State() string {
	return s.state
}

func (s Selector) Version() int {
	return s.version
}

func (s Selector) Get() (string, int, bool) {
	return s.state, s.version, s.set
}
