// Package testhelpers contains types and functions that may be useful in testing SDK functionality or
// custom integrations.
//
// Its subpackage storetest is meant to be used by any implementation of a persistent data store.
//
// The APIs in this package and its subpackages are supported as part of the SDK.
package testhelpers

// Implementation note: the types and functions in this package are mainly meant for external use, but may
// be useful in SDK tests. Anything that is *only* for SDK tests should be in internal/sharedtest instead.
// Avoid putting anything here that depends on any packages other than interfaces, since then it might not
// be possible to use it in other areas of the SDK without causing a cyclic reference (that's why storetest
// is a separate package, so it can reference the main package).
