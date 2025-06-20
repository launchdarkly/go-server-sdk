package ldplugins

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// Implementation Note: The Unimplemented struct is provided to simplify plugin implementation. It should always
// contain an implementation of all optional interfaces. It should not contain the Plugin interface directly
// because the implementer should be required to implement Metadata and Register.

// A Plugin is used to extend the functionality of the SDK.
//
// In order to avoid implementing unused methods, as well as easing maintenance of compatibility, implementors should
// compose the `Unimplemented`.
//
//	type MyPlugin struct {
//	  ldplugins.Unimplemented
//	}
type Plugin interface {
	Metadata() Metadata
	Register(client interfaces.LDClientInterface, metadata EnvironmentMetadata)
	HooksProvider
}

// HooksProvider is composed of methods that are called to retrieve hooks provided by the Plugin.
type HooksProvider interface {
	GetHooks(metadata EnvironmentMetadata) []ldhooks.Hook
}

// pluginInterfaces is an interface for implementation by the Unimplemented.
type pluginInterfaces interface {
	HooksProvider
}

// Unimplemented implements all Plugin methods with empty functions.
// Plugin implementors should use this to avoid having to implement empty methods and to ease updates when the Plugin
// interface is extended.
//
//	type MyPlugin struct {
//	  ldplugins.Unimplemented
//	}
//
// The plugin should implement Metadata and Register.
type Unimplemented struct {
}

// GetHooks is a default implementation of the GetHooks method.
func (p Unimplemented) GetHooks(_ EnvironmentMetadata) []ldhooks.Hook {
	return []ldhooks.Hook{}
}

// Implementation note: Unimplemented does not implement Metadata or Register because those must be implemented by
// plugin implementors.

// Ensure Unimplemented implements required interfaces.
var _ pluginInterfaces = Unimplemented{}
