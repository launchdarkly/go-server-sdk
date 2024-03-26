// Package ldotel contains OpenTelemetry specific implementations of hooks.
//
// For instance, to use LaunchDarkly with OpenTelemetry tracing, one would use the TracingHook:
//
// client, _ = ld.MakeCustomClient("sdk-47698c22-f258-4cd1-8e66-f2bd9bd1fc2a",
//
//	ld.Config{
//	    Hooks: []ldhooks.Hook{ldotel.NewTracingHook()},
//	}, 5*time.Second)
package ldotel
