// Package datastore is an internal package containing implementation types for the SDK's data store
// implementations (in-memory vs. cached persistent store) and related functionality. These types are
// not visible from outside of the SDK.
//
// This does not include implementations of specific database integrations such as Redis, which are in
// the ldconsul, lddynamodb, and ldredis packages.
package datastore
