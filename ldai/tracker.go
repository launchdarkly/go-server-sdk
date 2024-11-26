package ldai

type Tracker struct {
	config *Config
}

func NewTracker(config *Config) *Tracker {
	return &Tracker{config: config}
}
