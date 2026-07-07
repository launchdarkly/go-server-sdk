// Package ldoverrides provides sources for the SDK's flag override capability.
//
// Overrides are flag and segment definitions that take precedence over data received from
// LaunchDarkly at evaluation time, on a per-key basis. They exist for resilience during an
// incident: an operator can force one or more flags to a known state on a running
// application, whether or not the application can reach LaunchDarkly, and the override
// stays in effect until the operator removes it. Flags not present in the override data
// are completely unaffected.
//
// This package currently provides one source: FileSource, which reads overrides from local
// files and reloads them as the files change. Configure it with the data system builder:
//
//	config := ld.Config{
//	    DataSystem: ldcomponents.DataSystem().Default().
//	        Overrides(ldoverrides.FileSource().FilePaths("/etc/ld/overrides.json")),
//	}
//
// Evaluations served from an override are marked: the evaluation reason's IsOverride
// method reports true, and analytics events aggregate them into separate summary counters
// so they are distinguishable in LaunchDarkly.
package ldoverrides
