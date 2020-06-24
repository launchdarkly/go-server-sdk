// Package sharedtest contains types and functions used by SDK unit tests in multiple packages.
//
// Since it is inside internal/, none of this code can be seen by application code and it can be freely
// changed without breaking any public APIs. Test helpers that we want to be available to application code
// should be in testhelpers/ instead.
//
// It is important that no non-test code ever imports this package, so that it will not be compiled into
// applications as a transitive dependency.
//
// Note that this package is not allowed to reference the "internal" package, because the tests in that
// package use sharedtest helpers so it would be a circular reference.
package sharedtest
