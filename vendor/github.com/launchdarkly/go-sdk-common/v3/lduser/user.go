package lduser

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

// User is an alias for the type [ldcontext.Context], representing an evaluation context.
//
// This is provided as a compatibility helper for application code written for older SDK versions,
// which used users instead of contexts. See package comments for [lduser].
type User = ldcontext.Context

// UserAttribute is a string type representing the name of a user attribute.
//
// Constants like KeyAttribute describe all of the built-in attributes that existed in the older
// user model; you may also cast any string to UserAttribute when referencing a custom attribute
// name. In the newer context model, none of these attribute names except for "key", "name", and
// "anonymous" have any special significance.
type UserAttribute string

const (
	// KeyAttribute is the standard attribute name corresponding to UserBuilder.Key.
	KeyAttribute UserAttribute = "key"
	// SecondaryKeyAttribute is the standard attribute name corresponding to User.GetSecondaryKey().
	SecondaryKeyAttribute UserAttribute = "secondary"
	// IPAttribute is the standard attribute name corresponding to UserBuilder.IP.
	IPAttribute UserAttribute = "ip"
	// CountryAttribute is the standard attribute name corresponding to UserBuilder.Country.
	CountryAttribute UserAttribute = "country"
	// EmailAttribute is the standard attribute name corresponding to UserBuilder.Email.
	EmailAttribute UserAttribute = "email"
	// FirstNameAttribute is the standard attribute name corresponding to UserBuilder.FirstName.
	FirstNameAttribute UserAttribute = "firstName"
	// LastNameAttribute is the standard attribute name corresponding to UserBuilder.LastName.
	LastNameAttribute UserAttribute = "lastName"
	// AvatarAttribute is the standard attribute name corresponding to UserBuilder.Avatar.
	AvatarAttribute UserAttribute = "avatar"
	// NameAttribute is the standard attribute name corresponding to UserBuilder.Name.
	NameAttribute UserAttribute = "name"
	// AnonymousAttribute is the standard attribute name corresponding to UserBuilder.Anonymous.
	AnonymousAttribute UserAttribute = "anonymous"
)
