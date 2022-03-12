// Package ldservices provides HTTP handlers that simulate the behavior of LaunchDarkly service endpoints.
//
// This is mainly intended for use in the Go SDK's unit tests. It is also used in unit tests for the
// LaunchDarkly Relay Proxy, and could be useful in testing other applications that use the Go SDK if it
// is desirable to use real HTTP rather than other kinds of test fixtures.
package ldservices
