package lduser

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// NewUser creates a new user context identified by the given key.
//
// This is exactly equivalent to [ldcontext.New](key). It is provided to ease migration of code
// that previously used lduser instead of ldcontext.
func NewUser(key string) ldcontext.Context {
	return ldcontext.New(key)
}

// NewAnonymousUser creates a new anonymous user context identified by the given key.
//
// This is exactly equivalent to [ldcontext.NewBuilder](key).Anonymous(true).Build(). It is provided
// to ease migration of code that previously used lduser instead of ldcontext.
func NewAnonymousUser(key string) ldcontext.Context {
	return ldcontext.NewBuilder(key).Anonymous(true).Build()
}

// UserBuilder is a mutable object that uses the Builder pattern to specify properties for a user
// context.
//
// This is a compatibility helper that has been retained to ease migration of code from the older
// "user" model to the newer "context" model. See the package description of lduser for more
// about this.
//
// After obtaining an instance of UserBuilder by calling [NewUserBuilder], call setter methods such as
// UserBuilder.Name to specify any additional user properties. Then, call UserBuilder.Build to
// construct the [ldcontext.Context]. All of the UserBuilder setters return a reference the same builder,
// so they can be chained together:
//
//	context := NewUserBuilder("user-key").Name("Bob").Email("test@example.com").Build()
//
// Setters for attributes that can be designated private return the type
// [UserBuilderCanMakeAttributePrivate], so you can chain the AsPrivateAttribute method:
//
//	context := NewUserBuilder("user-key").Name("Bob").AsPrivateAttribute().Build() // Name is now private
//
// A UserBuilder should not be accessed by multiple goroutines at once.
//
// This is defined as an interface rather than a concrete type only for syntactic convenience (see
// [UserBuilderCanMakeAttributePrivate]). Applications should not implement this interface.
type UserBuilder interface {
	// Key changes the unique key for the user being built.
	Key(value string) UserBuilder

	// IP sets the IP address attribute for the user being built.
	IP(value string) UserBuilderCanMakeAttributePrivate

	// Country sets the country attribute for the user being built.
	Country(value string) UserBuilderCanMakeAttributePrivate

	// Email sets the email attribute for the user being built.
	Email(value string) UserBuilderCanMakeAttributePrivate

	// FirstName sets the first name attribute for the user being built.
	FirstName(value string) UserBuilderCanMakeAttributePrivate

	// LastName sets the last name attribute for the user being built.
	LastName(value string) UserBuilderCanMakeAttributePrivate

	// Avatar sets the avatar URL attribute for the user being built.
	Avatar(value string) UserBuilderCanMakeAttributePrivate

	// Name sets the full name attribute for the user being built.
	Name(value string) UserBuilderCanMakeAttributePrivate

	// Anonymous sets the Anonymous attribute for the user context being built.
	//
	// Anonymous means that the context will not be stored in the database that appears on your
	// LaunchDarkly dashboard. It does not imply that the context has no name; you can still set
	// Name or any other properties you want.
	Anonymous(value bool) UserBuilder

	// Custom sets a custom attribute for the user being built.
	//
	//     user := NewUserBuilder("user-key").
	//         Custom("custom-attr-name", ldvalue.String("some-string-value")).AsPrivateAttribute().
	//         Build()
	Custom(attribute string, value ldvalue.Value) UserBuilderCanMakeAttributePrivate

	// CustomAll sets all of the user's custom attributes at once from a ValueMap.
	//
	// UserBuilder has copy-on-write behavior to make this method efficient: if you do not make any
	// changes to custom attributes after this, it reuses the original map rather than allocating a
	// new one.
	CustomAll(ldvalue.ValueMap) UserBuilderCanMakeAttributePrivate

	// SetAttribute sets any attribute of the user being built, specified as a UserAttribute, to a value
	// of type ldvalue.Value.
	//
	// This method corresponds to the GetAttribute method of User. It is intended for cases where user
	// properties are being constructed generically, such as from a list of key-value pairs. Since not
	// all attributes have the same semantics, its behavior is as follows:
	//
	// 1. For built-in attributes, if the value is not of a type that is supported for that attribute,
	// the method has no effect. For Key, the only supported type is string; for Anonymous, the
	// supported types are boolean or null; and for all other built-ins, the supported types are
	// string or null. Custom attributes may be of any type.
	//
	// 2. Setting an attribute to null (ldvalue.Null() or ldvalue.Value{}) is the same as the attribute
	// not being set in the first place.
	//
	// 3. The method always returns the type UserBuilderCanMakeAttributePrivate, so that you can make
	// the attribute private if that is appropriate by calling AsPrivateAttribute(). For attributes
	// that cannot be made private (Key and Anonymous), calling AsPrivateAttribute() on this return
	// value will have no effect.
	SetAttribute(attribute UserAttribute, value ldvalue.Value) UserBuilderCanMakeAttributePrivate

	// Build creates a Context from the current UserBuilder properties.
	//
	// The Context is independent of the UserBuilder once you have called Build(); modifying the UserBuilder
	// will not affect an already-created Context.
	Build() ldcontext.Context
}

// UserBuilderCanMakeAttributePrivate is an extension of [UserBuilder] that allows attributes to be
// made private via UserBuilder.AsPrivateAttribute. All UserBuilderCanMakeAttributePrivate setter
// methods are the same as UserBuilder, and apply to the original builder.
//
// UserBuilder setter methods for attributes that can be made private always return this interface.
// See UserBuilder.AsPrivateAttribute for details.
type UserBuilderCanMakeAttributePrivate interface {
	UserBuilder

	// AsPrivateAttribute marks the last attribute that was set on this builder as being a private
	// attribute: that is, its value will not be sent to LaunchDarkly.
	//
	// This action only affects analytics events that are generated by this particular user object. To
	// mark some (or all) user attributes as private for all users, use the Config properties
	// PrivateAttributeName and AllAttributesPrivate.
	//
	// Most attributes can be made private, but Key and Anonymous cannot. This is enforced by the
	// compiler, since the builder methods for attributes that can be made private are the only ones
	// that return UserBuilderCanMakeAttributePrivate; therefore, you cannot write an expression like
	// NewUserBuilder("user-key").AsPrivateAttribute().
	//
	// In this example, FirstName and LastName are marked as private, but Country is not:
	//
	//     user := NewUserBuilder("user-key").
	//         FirstName("Pierre").AsPrivateAttribute().
	//         LastName("Menard").AsPrivateAttribute().
	//         Country("ES").
	//         Build()
	AsPrivateAttribute() UserBuilder

	// AsNonPrivateAttribute marks the last attribute that was set on this builder as not being a
	// private attribute: that is, its value will be sent to LaunchDarkly and can appear on the dashboard.
	//
	// This is the opposite of AsPrivateAttribute(), and has no effect unless you have previously called
	// AsPrivateAttribute() for the same attribute on the same user builder. For more details, see
	// AsPrivateAttribute().
	AsNonPrivateAttribute() UserBuilder
}

type userBuilderImpl struct {
	builder                     ldcontext.Builder
	lastAttributeCanMakePrivate string
}

// NewUserBuilder constructs a new UserBuilder, specifying the user key.
//
// For authenticated users, the key may be a username or e-mail address. For anonymous users,
// this could be an IP address or session ID.
func NewUserBuilder(key string) UserBuilder {
	b := &userBuilderImpl{}
	b.builder.Kind("user").Key(key)
	return b
}

// NewUserBuilderFromUser constructs a new UserBuilder, copying all attributes from an existing user. You may
// then call setter methods on the new UserBuilder to modify those attributes.
//
// Custom attributes, and the set of attribute names that are private, are implemented internally as maps.
// Since the User struct does not expose these maps, they are in effect immutable and will be reused from the
// original User rather than copied whenever possible. The UserBuilder has copy-on-write behavior so that it
// only makes copies of these data structures if you actually modify them.
func NewUserBuilderFromUser(fromUser ldcontext.Context) UserBuilder {
	return &userBuilderImpl{builder: *(ldcontext.NewBuilderFromContext(fromUser))}
}

func (b *userBuilderImpl) canMakeAttributePrivate(attribute string) UserBuilderCanMakeAttributePrivate {
	b.lastAttributeCanMakePrivate = attribute
	return b
}

func (b *userBuilderImpl) Key(value string) UserBuilder {
	b.builder.Key(value)
	return b
}

func (b *userBuilderImpl) IP(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("ip", value)
	return b.canMakeAttributePrivate(string(IPAttribute))
}

func (b *userBuilderImpl) Country(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("country", value)
	return b.canMakeAttributePrivate(string(CountryAttribute))
}

func (b *userBuilderImpl) Email(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("email", value)
	return b.canMakeAttributePrivate(string(EmailAttribute))
}

func (b *userBuilderImpl) FirstName(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("firstName", value)
	return b.canMakeAttributePrivate(string(FirstNameAttribute))
}

func (b *userBuilderImpl) LastName(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("lastName", value)
	return b.canMakeAttributePrivate(string(LastNameAttribute))
}

func (b *userBuilderImpl) Avatar(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("avatar", value)
	return b.canMakeAttributePrivate(string(AvatarAttribute))
}

func (b *userBuilderImpl) Name(value string) UserBuilderCanMakeAttributePrivate {
	b.builder.SetString("name", value)
	return b.canMakeAttributePrivate(string(NameAttribute))
}

func (b *userBuilderImpl) Anonymous(value bool) UserBuilder {
	b.builder.Anonymous(value)
	return b
}

func (b *userBuilderImpl) Custom(attribute string, value ldvalue.Value) UserBuilderCanMakeAttributePrivate {
	b.builder.SetValue(attribute, value)
	return b.canMakeAttributePrivate(attribute)
}

func (b *userBuilderImpl) CustomAll(valueMap ldvalue.ValueMap) UserBuilderCanMakeAttributePrivate {
	// CustomAll is defined as replacing all existing custom attributes. The context builder doesn't
	// have a method that applies to "all custom attributes" because it has a different notion of
	// what is custom than User does, so we need to use the following awkward logic.
	c := b.builder.Build()
	for _, name := range c.GetOptionalAttributeNames(nil) {
		switch name {
		case "secondary", "name", "firstName", "lastName", "email", "country", "avatar", "ip":
			continue
		default:
			b.builder.SetValue(name, ldvalue.Null())
		}
	}
	keys := make([]string, 0, 50) // arbitrary size to preallocate on stack
	for _, k := range valueMap.Keys(keys) {
		b.builder.SetValue(k, valueMap.Get(k))
	}
	b.lastAttributeCanMakePrivate = ""
	return b
}

func (b *userBuilderImpl) SetAttribute(
	attribute UserAttribute,
	value ldvalue.Value,
) UserBuilderCanMakeAttributePrivate {
	// The defined behavior of SetAttribute is that if it's used with the name of a built-in attribute
	// like key or name, it modifies that attribute if and only if the value is of a compatible type.
	// That's the same as the behavior of ldcontext.Builder.SetValue, except that UserBuilder also
	// supports setting Secondary by name-- and, UserBuilder enforces that formerly-built-in
	// attributes like Email can only be a string or null.
	switch attribute {
	case AnonymousAttribute:
		if value.IsBool() || value.IsNull() {
			b.builder.Anonymous(value.BoolValue())
		}
	case FirstNameAttribute, LastNameAttribute, EmailAttribute, CountryAttribute, AvatarAttribute, IPAttribute:
		if value.IsString() || value.IsNull() {
			b.builder.SetValue(string(attribute), value)
		}
	default:
		b.builder.SetValue(string(attribute), value)
	}
	if attribute != KeyAttribute && attribute != AnonymousAttribute {
		return b.canMakeAttributePrivate(string(attribute))
	}
	b.lastAttributeCanMakePrivate = ""
	return b
}

func (b *userBuilderImpl) Build() ldcontext.Context {
	return b.builder.Build()
}

func (b *userBuilderImpl) AsPrivateAttribute() UserBuilder {
	if b.lastAttributeCanMakePrivate != "" {
		b.builder.PrivateRef(ldattr.NewLiteralRef(b.lastAttributeCanMakePrivate))
	}
	return b
}

func (b *userBuilderImpl) AsNonPrivateAttribute() UserBuilder {
	if b.lastAttributeCanMakePrivate != "" {
		b.builder.RemovePrivateRef(ldattr.NewLiteralRef(b.lastAttributeCanMakePrivate))
	}
	return b
}
