// Package lduser defines the older LaunchDarkly SDK model for user properties.
//
// The SDK now uses the type [ldcontext.Context] to represent an evaluation context that might
// represent a user, or some other kind of entity, or multiple kinds. But in older SDK versions,
// this was limited to one kind and was represented by the type lduser.User. This differed from
// [ldcontext.Context] in several ways:
//   - There was always a single implicit context kind of "user" ([ldcontext.DefaultKind]).
//   - Unlike Context where only a few attributes such as Key and Name have special behavior, the
//     user model defined many other built-in attributes such as Email which, like Name, were constrained
//     to only allow string values. These had specific setter methods in [UserBuilder]. Non-built-in
//     attributes were considered "custom" attributes, and were enclosed in a "custom" object in JSON
//     representations.
//
// # The User type
//
// The SDK now operates only on Contexts, but the former [User] type is now an alias for
// [ldcontext.Context], so that some code written for the older SDK that referenced the User type
// can still work-- as long as it is treating the user as an opaque value and not trying to call
// methods of the old User type on it. For instance, the old User type had attribute accessors like
// GetEmail() which do not exist in [ldcontext.Context].
//
// # Updating code while still using UserBuilder
//
// The [UserBuilder] type has been retained and modified to be a wrapper for [ldcontext.Builder],
// allowing code that used the older model for building users to still work with minor adjustments.
//
// For any code that still uses UserBuilder, the significant differences from older SDK versions are:
//  1. The Build method of UserBuilder now returns an [ldcontext.Context], so you will need to update
//     any part of your code that referred to the lduser.User type by name.
//  2. The SDK no longer supports setting the key to an empty string. If you do this, the returned
//     Context will be invalid (as indicated by its [ldcontext.Context.Err] method returning an error) and the
//     SDK will refuse to use it for evaluations or events.
//  3. The SDK no longer supports setting the Secondary meta-attribute.
//  4. Previously, the Anonymous attribute of a user had three states: true, false, or undefined/null.
//     Undefined/null and false were functionally the same in terms of the LaunchDarkly
//     dashboard/indexing behavior, but they were represented differently in JSON and could behave
//     differently if referenced in a flag rule (an undefined/null value would not match "anonymous is
//     false"). Now, the prattributeoperty is a simple boolean defaulting to false, and the undefined state is
//     the same as false.
//
// # Migrating from User/UserBuilder to the ldcontext API
//
// It is preferable to update existing code to use the ldcontext package directly, rather than the
// [User] type alias and the [UserBuilder] wrapper. Here are the kinds of changes you may need to make:
//
// - Code that previously created a simple User with only a key should now use [ldcontext.New].
//
//	// old
//	user := lduser.NewUser("my-user-key")
//
//	// new
//	user := ldcontext.New("my-user-key")
//
// - Code that previously created a User with an empty string key ("") must be changed to use a
// non-empty key instead. If you do not care about the value of the key, use an arbitrary value.
// If you do not want the key to appear on your LaunchDarkly dashboard, set the Anonymous attribute.
//
// - Code that previously used UserBuilder should now use [ldcontext.NewBuilder].
//
// - The [ldcontext.Builder] has fewer attribute-name-specific setter methods: [ldcontext.Builder.Name] is
// still a built-in attribute with its own setter, but for all other optional attributes such as Email
// that you are setting to a string value, you should instead call [ldcontext.Builder.SetString] and
// specify the attribute name as the first parameter.
//
//	// old
//	user := lduser.NewUserBuilder("my-user-key").
//	    Name("my-name").
//	    Email("my-email").
//	    Build()
//
//	// new
//	user := ldcontext.NewBuilder("my-user-key").
//	    Name("my-name").
//	    SetString("email", "my-email").
//	    Build()
//
// - The SetCustom method has been replaced by several Set methods for specific value types,
// and the [ldcontext.Builder.SetValue] method which takes an ldvalue.Value representing a value of any
// type (boolean, number, string, array, or object).
//
//	// old
//	user := lduser.NewUserBuilder("my-user-key").
//	    Custom("my-string-attr", ldvalue.String("value")).
//	    Custom("my-array-attr", ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))).
//	    Build()
//
//	// new
//	user := ldcontext.NewBuilder("my-user-key").
//	    SetString("my-string-attr", "value").
//	    Set("my-array-attr", ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))).
//	    Build()
//
// - Private attributes are now designated by attribute name with [ldcontext.Builder.Private],
// instead of chaining a call to AsPrivateAttribute() after calling the setter.
//
//	// old
//	user := lduser.NewUserBuilder("my-user-key").
//	    Name("my-name").AsPrivateAttribute().
//	    Email("my-email").AsPrivateAttribute().
//	    Build()
//
//	// new
//	user := ldcontext.NewBuilder("my-user-key").
//	    Name("my-name").
//	    SetString("email", "my-email").
//	    Private("name", "email").
//	    Build()
package lduser
