package fdv2proto

type Selector struct {
	state   string
	version int
}

func NoSelector() *Selector {
	return nil
}

func NewSelector(state string, version int) *Selector {
	return &Selector{state: state, version: version}
}

func (s *Selector) IsSet() bool {
	return s != nil
}

func (s *Selector) State() string {
	return s.state
}

func (s *Selector) Version() int {
	return s.version
}
