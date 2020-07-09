// Package ldclient is the main package for the LaunchDarkly SDK.
//
// This package contains the types and methods for the SDK client (LDClient) and its overall
// configuration.
//
// Subpackages in the same repository provide additional functionality for specific features of the
// client. Most applications that need to change any configuration settings will use the ldcomponents
// package (https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents).
//
// The SDK also uses types from the go-sdk-common.v2 repository and its subpackages
// (https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2) that represent standard data structures
// in the LaunchDarkly model. All applications that evaluate feature flags will use the lduser
// package (https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/lduser); for some features such
// as custom attributes, the ldvalue package is also helpful.
//
// For more information and code examples, see the Go SDK Reference:
// https://docs.launchdarkly.com/sdk/server-side/go
package ldclient
