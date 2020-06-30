// Package datakinds contains the implementations of ldstoretypes.DataKind for flags and segments.
//
// These have their own package because they are used in many places and therefore need to be in a
// package that doesn't have other things with other dependencies, to avoid cyclic references.
package datakinds
