// Package sharedtest contains types and functions used by SDK unit tests in multiple packages.
//
// Application code should not rely on anything in this package; it is not supported as part of the SDK.
// The one exception is that external implementations of PersistentDataStore can and should be tested by
// using PersistentDataStoreTestSuite.
package sharedtest
