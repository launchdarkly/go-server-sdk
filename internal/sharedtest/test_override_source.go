package sharedtest

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// TestOverrideSource is a simple programmatic OverrideSource for testing the override
// system without any file machinery. It also serves as a reference for the seam an
// override source implements: Start delivers the initial data synchronously, and
// SetOverrides pushes replacement snapshots to the sink at any time afterward.
type TestOverrideSource struct {
	mu          sync.Mutex
	sink        subsystems.OverrideSink
	initialData []ldstoretypes.Collection
	closed      bool
}

var _ subsystems.OverrideSource = (*TestOverrideSource)(nil)

// NewTestOverrideSource creates a TestOverrideSource that will supply the given data as
// soon as it is started. A nil initialData means the source supplies nothing until
// SetOverrides is called.
func NewTestOverrideSource(initialData []ldstoretypes.Collection) *TestOverrideSource {
	return &TestOverrideSource{initialData: initialData}
}

// Start supplies the initial data, if any, before returning.
func (t *TestOverrideSource) Start(sink subsystems.OverrideSink) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sink = sink
	if t.initialData != nil {
		sink.SetOverrides(t.initialData)
	}
}

// SetOverrides replaces the override layer contents, as if the source's backing data had
// changed. It must not be called before the source is started.
func (t *TestOverrideSource) SetOverrides(data []ldstoretypes.Collection) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sink != nil && !t.closed {
		t.sink.SetOverrides(data)
	}
}

// Close marks the source closed; subsequent SetOverrides calls are ignored.
func (t *TestOverrideSource) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

// IsClosed reports whether Close has been called.
func (t *TestOverrideSource) IsClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// IsStarted reports whether Start has been called.
func (t *TestOverrideSource) IsStarted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sink != nil
}

// Build implements ComponentConfigurer, so the source can be used directly in the
// data system configuration builder's Overrides method.
func (t *TestOverrideSource) Build(context subsystems.ClientContext) (subsystems.OverrideSource, error) {
	return t, nil
}
