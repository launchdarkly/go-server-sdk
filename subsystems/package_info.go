// Package subsystems contains interfaces for implementation of custom LaunchDarkly components.
//
// Most applications will not need to refer to these types. You will use them if you are creating a
// plug-in component, such as a database integration, or a test fixture. They are also used as
// interfaces for the built-in SDK components, so that plugin components can be used interchangeably
// with those: for instance, Config.DataStore uses the type subsystems.DataStore as an abstraction
// for the data store component.
//
// The package also includes concrete types that are used as parameters within these interfaces.
package subsystems
