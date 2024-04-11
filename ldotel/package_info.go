// Package ldotel contains OpenTelemetry specific implementations of hooks.
//
// For instance, to use LaunchDarkly with OpenTelemetry tracing, one would use the TracingHook:
//
//	client, _ = ld.MakeCustomClient("sdk-key", ld.Config{
//		    Hooks: []ldhooks.Hook{ldotel.NewTracingHook()},
//		}, 5*time.Second)
package ldotel

// Version is the current version string of the ldotel package. This is updated by our release scripts.
const Version = "1.0.0" // {{ x-release-please-version }}
