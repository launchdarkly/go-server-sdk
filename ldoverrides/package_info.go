// Package ldoverrides provides sources for the SDK's flag override capability.
//
// Overrides are flag and segment definitions that take precedence over data received from
// LaunchDarkly at evaluation time, on a per-key basis. They exist for resilience during an
// incident. An operator can force one or more flags to a known state on a running
// application, whether or not the application can reach LaunchDarkly. The override stays in
// effect until the operator removes it. Flags not present in the override data are
// completely unaffected.
//
// This package currently provides one source: FileSource, which reads overrides from local
// files and reloads them as the files change. Configure it with the data system builder:
//
//	config := ld.Config{
//	    DataSystem: ldcomponents.DataSystem().Default().
//	        Overrides(ldoverrides.FileSource().FilePaths("/etc/ld/overrides.json")),
//	}
//
// An evaluation that an override affects is marked. The marking is direct or transitive. It
// applies when the evaluated flag, a prerequisite at any depth, or a segment read during the
// evaluation came from the override layer. The evaluation reason's IsOverrideAffected method
// reports the marking. Marked evaluations appear in analytics summary events only, under
// separate counters, so LaunchDarkly can distinguish them. They produce no individual
// evaluation events.
package ldoverrides
