package datasystem

import (
	"context"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A TTL that is valid, so that it is used as-is, and short, so that recovery is quick.
const shortFallbackTTL = 10 * time.Millisecond

// lifecycleSynchronizer is a DataSynchronizer that reports when it starts and closes. Each
// build of a lifecycleBuilder creates a new one.
type lifecycleSynchronizer struct {
	name    string
	results chan subsystems.DataSynchronizerResult
	events  chan<- string
}

func (s *lifecycleSynchronizer) Name() string { return s.name }

func (s *lifecycleSynchronizer) Fetch(
	_ subsystems.DataSelector,
	_ context.Context,
) (*subsystems.Basis, subsystems.FDv1Fallback, error) {
	return nil, subsystems.FDv1Fallback{}, nil
}

func (s *lifecycleSynchronizer) Sync(_ subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	s.events <- s.name + " started"
	return s.results
}

func (s *lifecycleSynchronizer) Close() error {
	s.events <- s.name + " closed"
	return nil
}

type lifecycleBuilder struct {
	name   string
	events chan string
	built  chan *lifecycleSynchronizer
}

func newLifecycleBuilder(name string, events chan string) *lifecycleBuilder {
	return &lifecycleBuilder{name: name, events: events, built: make(chan *lifecycleSynchronizer, 10)}
}

func (b *lifecycleBuilder) build() (subsystems.DataSynchronizer, error) {
	s := &lifecycleSynchronizer{
		name:    b.name,
		results: make(chan subsystems.DataSynchronizerResult, 1),
		events:  b.events,
	}
	b.built <- s
	return s, nil
}

// fallbackInitializer is a DataInitializer that requests an FDv1 fallback.
type fallbackInitializer struct {
	ttl time.Duration
}

func (i fallbackInitializer) Name() string { return "fallback-initializer" }

func (i fallbackInitializer) Fetch(
	_ subsystems.DataSelector,
	_ context.Context,
) (*subsystems.Basis, subsystems.FDv1Fallback, error) {
	return nil, subsystems.FDv1Fallback{Requested: true, TTL: i.ttl}, nil
}

func makeRecoveryTestFDv2(
	t *testing.T,
	loggers ldlog.Loggers,
	initializers []subsystems.DataInitializer,
	fdv2 *lifecycleBuilder,
	fdv1 *lifecycleBuilder,
) *FDv2 {
	t.Helper()
	synchronizers := subsystems.SynchronizersConfiguration{
		SynchronizerBuilders: []func() (subsystems.DataSynchronizer, error){fdv2.build},
	}
	if fdv1 != nil {
		synchronizers.FDv1FallbackBuilder = fdv1.build
	}

	clientContext := &internal.ClientContextImpl{
		BasicClientContext: subsystems.BasicClientContext{
			SDKKey:  sharedtest.TestSDKKey,
			Logging: subsystems.LoggingConfiguration{Loggers: loggers},
		},
	}

	system, err := NewFDv2(false, fixedConfig{
		Initializers:  initializers,
		Synchronizers: synchronizers,
	}, clientContext, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = system.Stop() })
	return system
}

func requireEvents(t *testing.T, events <-chan string, want ...string) {
	t.Helper()
	for _, w := range want {
		assert.Equal(t, w, th.RequireValue(t, events, time.Second, "timed out waiting for %q", w))
	}
}

func TestFDv1FallbackDurationUsesValidTTLAsIs(t *testing.T) {
	for _, ttl := range []time.Duration{time.Millisecond, time.Second, 120 * time.Second, time.Hour} {
		assert.Equal(t, ttl, fdv1FallbackDuration(ttl), ttl.String())
	}
}

func TestFDv1FallbackDurationUsesJitteredDefaultForInvalidTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second, time.Hour + time.Second, 24 * time.Hour} {
		for i := 0; i < 100; i++ {
			duration := fdv1FallbackDuration(ttl)
			assert.GreaterOrEqual(t, duration, 30*time.Minute, ttl.String())
			assert.LessOrEqual(t, duration, time.Hour, ttl.String())
		}
	}
}

func TestFDv2RecoversAfterSynchronizerFallbackWithoutFDv1Synchronizer(t *testing.T) {
	events := make(chan string, 10)
	fdv2 := newLifecycleBuilder("fdv2", events)

	system := makeRecoveryTestFDv2(t, ldlog.NewDisabledLoggers(), nil, fdv2, nil)
	system.Start(make(chan struct{}))

	first := th.RequireValue(t, fdv2.built, time.Second)
	first.results <- subsystems.DataSynchronizerResult{
		State:           interfaces.DataSourceStateOff,
		FallbackToFDv1:  true,
		FDv1FallbackTTL: shortFallbackTTL,
	}

	// The FDv2 synchronizer is closed and, after the TTL, started again.
	requireEvents(t, events, "fdv2 started", "fdv2 closed", "fdv2 started")
	assert.Equal(t, interfaces.DataSourceStateInterrupted, system.getStatus().State)
}

func TestFDv2RecoversAfterSynchronizerFallbackToFDv1Synchronizer(t *testing.T) {
	events := make(chan string, 10)
	fdv2 := newLifecycleBuilder("fdv2", events)
	fdv1 := newLifecycleBuilder("fdv1", events)

	system := makeRecoveryTestFDv2(t, ldlog.NewDisabledLoggers(), nil, fdv2, fdv1)
	system.Start(make(chan struct{}))

	first := th.RequireValue(t, fdv2.built, time.Second)
	first.results <- subsystems.DataSynchronizerResult{
		State:           interfaces.DataSourceStateValid,
		FallbackToFDv1:  true,
		FDv1FallbackTTL: shortFallbackTTL,
	}

	// Each synchronizer is closed before the next one starts, so FDv1 and FDv2 never write to
	// the store at the same time.
	requireEvents(t, events, "fdv2 started", "fdv2 closed", "fdv1 started", "fdv1 closed", "fdv2 started")
}

func TestFDv2RecoversAfterInitializerFallbackWithoutFDv1Synchronizer(t *testing.T) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)

	events := make(chan string, 10)
	fdv2 := newLifecycleBuilder("fdv2", events)

	system := makeRecoveryTestFDv2(t, mockLog.Loggers,
		[]subsystems.DataInitializer{fallbackInitializer{ttl: shortFallbackTTL}}, fdv2, nil)
	system.Start(make(chan struct{}))

	// The FDv2 synchronizer only starts once the TTL has expired.
	requireEvents(t, events, "fdv2 started")
	assert.True(t, mockLog.HasMessageMatch(ldlog.Info, "FDv1 fallback TTL has expired"))
}
