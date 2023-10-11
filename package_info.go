// Package ldclient is the main package for the LaunchDarkly SDK.
//
// This package contains the types and methods for the SDK client ([LDClient]) and its overall
// configuration ([Config]).
//
// Subpackages in the same repository provide additional functionality for specific features of the
// client. Most applications that need to change any configuration settings will use the package
// [github.com/launchdarkly/go-server-sdk/v7/ldcomponents].
//
// The SDK also uses types from the go-sdk-common repository and its subpackages
// ([github.com/launchdarkly/go-sdk-common/v3) that represent standard data structures
// in the LaunchDarkly model. All applications that evaluate feature flags will use the ldcontext
// package ([github.com/launchdarkly/go-sdk-common/v3/ldcontext]); for some features such
// as custom attributes with complex data types, the ldvalue package is also helpful
// ([github.com/launchdarkly/go-sdk-common/v3/ldvalue]).
//
// For more information and code examples, see the Go SDK Reference:
// https://docs.launchdarkly.com/sdk/server-side/go
package ldclient
