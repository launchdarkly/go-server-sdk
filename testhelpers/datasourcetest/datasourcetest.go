// Package datasourcetest provides test-only helpers for configuring knobs on the
// SDK's streaming and polling data source builders that are not part of the
// SDK's stable public API. It is intended for LaunchDarkly's own contract-test
// tooling and for SDK integration tests that need to observe extended-regime
// behavior within a test-relevant time budget.
//
// Production code must not import this package.
//
// This package is a companion to the Internal() escape hatches on the builders
// in ldcomponents. Adding a new test-only knob to a builder means:
//  1. Add the field and the setter to the <Builder>Internal type in ldcomponents.
//  2. Add a corresponding free-function helper here that delegates to it.
package datasourcetest

import (
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
)

// WithStreamingExtendedInitialReconnectDelay overrides the RETRY-spec default
// base delay for the streaming extended-regime retry curve. Test-only.
func WithStreamingExtendedInitialReconnectDelay(
	b *ldcomponents.StreamingDataSourceBuilder,
	delay time.Duration,
) *ldcomponents.StreamingDataSourceBuilder {
	b.Internal().ExtendedInitialReconnectDelay(delay)
	return b
}

// WithStreamingRetryResetInterval overrides the threshold of continuous healthy
// stream operation before the SDK resets its retry backoff to the normal regime.
// Test-only.
func WithStreamingRetryResetInterval(
	b *ldcomponents.StreamingDataSourceBuilder,
	interval time.Duration,
) *ldcomponents.StreamingDataSourceBuilder {
	b.Internal().RetryResetInterval(interval)
	return b
}

// WithPollingExtendedInitialPollInterval overrides the RETRY-spec default base
// delay for the polling extended-regime backoff. Test-only.
func WithPollingExtendedInitialPollInterval(
	b *ldcomponents.PollingDataSourceBuilder,
	delay time.Duration,
) *ldcomponents.PollingDataSourceBuilder {
	b.Internal().ExtendedInitialPollInterval(delay)
	return b
}
