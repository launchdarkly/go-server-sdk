package ldclient

import (
	"encoding/json"
	"time"
)

// A User contains specific attributes of a user browsing your site. The only mandatory property property is the Key,
// which must uniquely identify each user. For authenticated users, this may be a username or e-mail address. For anonymous users,
// this could be an IP address or session ID.
//
// Besides the mandatory Key, User supports two kinds of optional attributes: interpreted attributes (e.g. Ip and Country)
// and custom attributes.  LaunchDarkly can parse interpreted attributes and attach meaning to them. For example, from an IP address, LaunchDarkly can
// do a geo IP lookup and determine the user's country.
//
// Custom attributes are not parsed by LaunchDarkly. They can be used in custom rules-- for example, a custom attribute such as "customer_ranking" can be used to
// launch a feature to the top 10% of users on a site.
//
// User fields will be made private in the future, accessible only via getter methods, to prevent unsafe
// modification of users after they are created. The preferred method of constructing a User is to use either
// a simple constructor (NewUser, NewAnonymousUser) or the builder pattern with NewUserBuilder. If you do set
// the User fields directly, it is important not to change any map/slice elements, and not change a string
// that is pointed to by an existing pointer, after the User has been passed to any SDK methods; otherwise,
// flag evaluations and analytics events may refer to the wrong user properties (or, in the case of a map, you
// may even cause a concurrent modification panic).
type User struct {
	// Key is the unique key of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Key *string `json:"key,omitempty" bson:"key,omitempty"`
	// SecondaryKey is the secondary key of the user.
	//
	// This affects feature flag targeting (https://docs.launchdarkly.com/docs/targeting-users#section-targeting-rules-based-on-user-attributes)
	// as follows: if you have chosen to bucket users by a specific attribute, the secondary key (if set)
	// is used to further distinguish between users who are otherwise identical according to that attribute.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Secondary *string `json:"secondary,omitempty" bson:"secondary,omitempty"`
	// Ip is the IP address attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Ip *string `json:"ip,omitempty" bson:"ip,omitempty"`
	// Country is the country attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Country *string `json:"country,omitempty" bson:"country,omitempty"`
	// Email is the email address attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Email *string `json:"email,omitempty" bson:"email,omitempty"`
	// FirstName is the first name attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	FirstName *string `json:"firstName,omitempty" bson:"firstName,omitempty"`
	// LastName is the last name attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	LastName *string `json:"lastName,omitempty" bson:"lastName,omitempty"`
	// Avatar is the avatar URL attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Avatar *string `json:"avatar,omitempty" bson:"avatar,omitempty"`
	// Name is the name attribute of the user.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Name *string `json:"name,omitempty" bson:"name,omitempty"`
	// Anonymous indicates whether the user is anonymous.
	//
	// If a user is anonymous, the user key will not appear on your LaunchDarkly dashboard.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Anonymous *bool `json:"anonymous,omitempty" bson:"anonymous,omitempty"`
	// Custom is the user's map of custom attribute names and values.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Custom *map[string]interface{} `json:"custom,omitempty" bson:"custom,omitempty"`
	// Derived is used internally by the SDK.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	Derived map[string]*DerivedAttribute `json:"derived,omitempty" bson:"derived,omitempty"`

	// PrivateAttributes contains a list of attribute names that were included in the user,
	// but were marked as private. As such, these attributes are not included in the fields above.
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	PrivateAttributes []string `json:"privateAttrs,omitempty" bson:"privateAttrs,omitempty"`

	// This contains list of attributes to keep private, whether they appear at the top-level or Custom
	// The attribute "key" is always sent regardless of whether it is in this list, and "custom" cannot be used to
	// eliminate all custom attributes
	//
	// Deprecated: Direct access to User fields is now deprecated in favor of UserBuilder. In a future version,
	// User fields will be private and only accessible via getter methods.
	PrivateAttributeNames []string `json:"-" bson:"-"`
}

// GetKey gets the unique key of the user.
func (u User) GetKey() string {
	// Key is only nullable for historical reasons - all users should have a key
	if u.Key == nil {
		return ""
	}
	return *u.Key
}

// GetSecondaryKey returns the secondary key of the user, if any.
//
// This affects feature flag targeting (https://docs.launchdarkly.com/docs/targeting-users#section-targeting-rules-based-on-user-attributes)
// as follows: if you have chosen to bucket users by a specific attribute, the secondary key (if set)
// is used to further distinguish between users who are otherwise identical according to that attribute.
func (u User) GetSecondaryKey() OptionalString {
	return NewOptionalStringFromPointer(u.Secondary)
}

// GetIP() returns the IP address attribute of the user, if any.
func (u User) GetIP() OptionalString {
	return NewOptionalStringFromPointer(u.Ip)
}

// GetCountry() returns the country attribute of the user, if any.
func (u User) GetCountry() OptionalString {
	return NewOptionalStringFromPointer(u.Country)
}

// GetEmail() returns the email address attribute of the user, if any.
func (u User) GetEmail() OptionalString {
	return NewOptionalStringFromPointer(u.Email)
}

// GetFirstName() returns the first name attribute of the user, if any.
func (u User) GetFirstName() OptionalString {
	return NewOptionalStringFromPointer(u.FirstName)
}

// GetLastName() returns the last name attribute of the user, if any.
func (u User) GetLastName() OptionalString {
	return NewOptionalStringFromPointer(u.LastName)
}

// GetAvatar() returns the avatar URL attribute of the user, if any.
func (u User) GetAvatar() OptionalString {
	return NewOptionalStringFromPointer(u.Avatar)
}

// GetName() returns the full name attribute of the user, if any.
func (u User) GetName() OptionalString {
	return NewOptionalStringFromPointer(u.Name)
}

// GetAnonymous() returns the anonymous attribute of the user.
//
// If a user is anonymous, the user key will not appear on your LaunchDarkly dashboard.
func (u User) GetAnonymous() bool {
	return u.Anonymous != nil && *u.Anonymous
}

// GetAnonymousOptional() returns the anonymous attribute of the user, with a second value indicating
// whether that attribute was defined for the user or not.
func (u User) GetAnonymousOptional() (bool, bool) {
	return u.GetAnonymous(), u.Anonymous != nil
}

// GetCustom() returns a custom attribute of the user by name. The boolean second return value indicates
// whether any value was set for this attribute or not.
//
// A custom attribute value can be of any type supported by JSON: boolean, number, string, array
// (slice), or object (map).
//
// Since slices and maps are passed by reference, it is important that you not modify any elements of
// a slice or map that is in a user custom attribute, because the SDK might simultaneously be trying to
// access it on another goroutine.
func (u User) GetCustom(attrName string) (interface{}, bool) {
	if u.Custom == nil {
		return nil, false
	}
	value, found := (*u.Custom)[attrName]
	return value, found
}

// GetCustomKeys() returns the keys of all custom attributes that have been set on this user.
func (u User) GetCustomKeys() []string {
	if u.Custom == nil || len(*u.Custom) == 0 {
		return nil
	}
	keys := make([]string, 0, len(*u.Custom))
	for key := range *u.Custom {
		keys = append(keys, key)
	}
	return keys
}

// Equal tests whether two users have equal attributes.
//
// Regular struct equality comparison is not allowed for User because it can contain slices and
// maps. This method is faster than using reflect.DeepEqual(), and also correctly ignores
// insignificant differences in the internal representation of the attributes.
func (u User) Equal(other User) bool {
	if u.GetKey() != other.GetKey() ||
		u.GetSecondaryKey() != other.GetSecondaryKey() ||
		u.GetIP() != other.GetIP() ||
		u.GetCountry() != other.GetCountry() ||
		u.GetEmail() != other.GetEmail() ||
		u.GetFirstName() != other.GetFirstName() ||
		u.GetLastName() != other.GetLastName() ||
		u.GetAvatar() != other.GetAvatar() ||
		u.GetName() != other.GetName() ||
		u.GetAnonymous() != other.GetAnonymous() {
		return false
	}
	if (u.Anonymous == nil) != (other.Anonymous == nil) ||
		u.Anonymous != nil && *u.Anonymous != *other.Anonymous {
		return false
	}
	if (u.Custom == nil) != (other.Custom == nil) ||
		u.Custom != nil && len(*u.Custom) != len(*other.Custom) {
		return false
	}
	if u.Custom != nil {
		for k, v := range *u.Custom {
			v1, ok := (*other.Custom)[k]
			if !ok || v != v1 {
				return false
			}
		}
	}
	if !stringSlicesEqual(u.PrivateAttributeNames, other.PrivateAttributeNames) {
		return false
	}
	if !stringSlicesEqual(u.PrivateAttributes, other.PrivateAttributes) {
		return false
	}
	return true
}

// String returns a simple string representation of a user.
func (u User) String() string {
	bytes, _ := json.Marshal(u)
	return string(bytes)
}

// Used internally in evaluations.
func (u User) valueOf(attr string) (interface{}, bool) {
	if attr == "key" {
		if u.Key != nil {
			return *u.Key, true
		}
		return nil, false
	} else if attr == "ip" {
		return u.GetIP().asEmptyInterface()
	} else if attr == "country" {
		return u.GetCountry().asEmptyInterface()
	} else if attr == "email" {
		return u.GetEmail().asEmptyInterface()
	} else if attr == "firstName" {
		return u.GetFirstName().asEmptyInterface()
	} else if attr == "lastName" {
		return u.GetLastName().asEmptyInterface()
	} else if attr == "avatar" {
		return u.GetAvatar().asEmptyInterface()
	} else if attr == "name" {
		return u.GetName().asEmptyInterface()
	} else if attr == "anonymous" {
		value, ok := u.GetAnonymousOptional()
		return value, ok
	}

	// Select a custom attribute
	value, ok := u.GetCustom(attr)
	return value, ok
}

// DerivedAttribute is an entry in a Derived attribute map and is for internal use by LaunchDarkly only. Derived attributes
// sent to LaunchDarkly are ignored.
//
// Deprecated: this type is for internal use and will be removed in a future version.
type DerivedAttribute struct {
	Value       interface{} `json:"value" bson:"value"`
	LastDerived time.Time   `json:"lastDerived" bson:"lastDerived"`
}

// NewUser creates a new user identified by the given key.
func NewUser(key string) User {
	return User{Key: &key}
}

// NewAnonymousUser creates a new anonymous user identified by the given key.
func NewAnonymousUser(key string) User {
	anonymous := true
	return User{Key: &key, Anonymous: &anonymous}
}

// UserBuilder is a mutable struct that uses the Builder pattern to specify properties for a User.
// This is the preferred method for constructing a User; direct access to User fields will be
// removed in a future version.
//
// Obtain an instance of UserBuilder by calling NewUserBuilder, then call setter methods such as
// Name to specify any additional user properties, then call Build() to construct the User. All of
// the UserBuilder setters return a reference the same builder, so they can be chained together:
//
//     user := NewUserBuilder("user-key").Name("Bob").Email("test@example.com").Build()
//
// Setters for user attributes that can be designated private return the type
// UserBuilderCanMakeAttributePrivate, so you can chain the AsPrivateAttribute method:
//
//     user := NewUserBuilder("user-key").Name("Bob").AsPrivateAttribute().Build() // Name is now private
//
// A UserBuilder should not be accessed by multiple goroutines at once.
type UserBuilder interface {
	Key(value string) UserBuilder
	Secondary(value string) UserBuilderCanMakeAttributePrivate
	IP(value string) UserBuilderCanMakeAttributePrivate
	Country(value string) UserBuilderCanMakeAttributePrivate
	Email(value string) UserBuilderCanMakeAttributePrivate
	FirstName(value string) UserBuilderCanMakeAttributePrivate
	LastName(value string) UserBuilderCanMakeAttributePrivate
	Avatar(value string) UserBuilderCanMakeAttributePrivate
	Name(value string) UserBuilderCanMakeAttributePrivate
	Anonymous(value bool) UserBuilder
	Custom(name string, value interface{}) UserBuilderCanMakeAttributePrivate
	Build() User
}

// UserBuilderCanMakeAttributePrivate is an extension of UserBuilder that allows attributes to be
// made private via the AsPrivateAttribute() method. All UserBuilderCanMakeAttributePrivate setter
// methods are the same as UserBuilder, and apply to the original builder.
//
// UserBuilder setter methods for attributes that can be made private always return this interface.
// See AsPrivateAttribute for details.
type UserBuilderCanMakeAttributePrivate interface {
	UserBuilder
	AsPrivateAttribute() UserBuilder
}

type userBuilderImpl struct {
	key          string
	secondary    OptionalString
	ip           OptionalString
	country      OptionalString
	email        OptionalString
	firstName    OptionalString
	lastName     OptionalString
	avatar       OptionalString
	name         OptionalString
	anonymous    bool
	hasAnonymous bool
	custom       map[string]interface{}
	privateAttrs map[string]bool
}

type userBuilderCanMakeAttributePrivate struct {
	builder  *userBuilderImpl
	attrName string
}

// NewUserBuilder constructs a new UserBuilder, specifying the user key.
//
// For authenticated users, the key may be a username or e-mail address. For anonymous users,
// this could be an IP address or session ID.
func NewUserBuilder(key string) UserBuilder {
	return &userBuilderImpl{key: key}
}

// NewUserBuilderFromUser constructs a new UserBuilder, copying all attributes from an existing user. You may
// then call setter methods on the new UserBuilder to modify those attributes.
func NewUserBuilderFromUser(fromUser User) UserBuilder {
	builder := &userBuilderImpl{
		secondary: fromUser.GetSecondaryKey(),
		ip:        fromUser.GetIP(),
		country:   fromUser.GetCountry(),
		email:     fromUser.GetEmail(),
		firstName: fromUser.GetFirstName(),
		lastName:  fromUser.GetLastName(),
		avatar:    fromUser.GetAvatar(),
		name:      fromUser.GetName(),
	}
	if fromUser.Key != nil {
		builder.key = *fromUser.Key
	}
	if fromUser.Anonymous != nil {
		builder.anonymous = *fromUser.Anonymous
		builder.hasAnonymous = true
	}
	if fromUser.Custom != nil {
		builder.custom = make(map[string]interface{}, len(*fromUser.Custom))
		for k, v := range *fromUser.Custom {
			builder.custom[k] = v
		}
	}
	if len(fromUser.PrivateAttributeNames) > 0 {
		builder.privateAttrs = make(map[string]bool, len(fromUser.PrivateAttributeNames))
		for _, name := range fromUser.PrivateAttributeNames {
			builder.privateAttrs[name] = true
		}
	}
	return builder
}

func (b *userBuilderImpl) canMakeAttributePrivate(attrName string) UserBuilderCanMakeAttributePrivate {
	return &userBuilderCanMakeAttributePrivate{builder: b, attrName: attrName}
}

// Key changes the unique key for the user being built.
func (b *userBuilderImpl) Key(value string) UserBuilder {
	b.key = value
	return b
}

// Secondary sets the secondary key attribute for the user being built.
//
// This affects feature flag targeting (https://docs.launchdarkly.com/docs/targeting-users#section-targeting-rules-based-on-user-attributes)
// as follows: if you have chosen to bucket users by a specific attribute, the secondary key (if set)
// is used to further distinguish between users who are otherwise identical according to that attribute.
func (b *userBuilderImpl) Secondary(value string) UserBuilderCanMakeAttributePrivate {
	b.secondary = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("secondary")
}

// IP sets the IP address attribute for the user being built.
func (b *userBuilderImpl) IP(value string) UserBuilderCanMakeAttributePrivate {
	b.ip = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("ip")
}

// IP sets the country attribute for the user being built.
func (b *userBuilderImpl) Country(value string) UserBuilderCanMakeAttributePrivate {
	b.country = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("country")
}

// IP sets the country attribute for the user being built.
func (b *userBuilderImpl) Email(value string) UserBuilderCanMakeAttributePrivate {
	b.email = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("email")
}

// FirstName sets the first name attribute for the user being built.
func (b *userBuilderImpl) FirstName(value string) UserBuilderCanMakeAttributePrivate {
	b.firstName = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("firstName")
}

// LastName sets the last name attribute for the user being built.
func (b *userBuilderImpl) LastName(value string) UserBuilderCanMakeAttributePrivate {
	b.lastName = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("lastName")
}

// Avatar sets the avatar URL attribute for the user being built.
func (b *userBuilderImpl) Avatar(value string) UserBuilderCanMakeAttributePrivate {
	b.avatar = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("avatar")
}

// Name sets the full name attribute for the user being built.
func (b *userBuilderImpl) Name(value string) UserBuilderCanMakeAttributePrivate {
	b.name = NewOptionalStringWithValue(value)
	return b.canMakeAttributePrivate("name")
}

// Anonymous sets the anonymous attribute for the user being built.
//
// If a user is anonymous, the user key will not appear on your LaunchDarkly dashboard.
func (b *userBuilderImpl) Anonymous(value bool) UserBuilder {
	b.anonymous = value
	b.hasAnonymous = true
	return b
}

// Custom sets a custom attribute for the user being built.
//
// A custom attribute value can be of any type supported by JSON: boolean, number, string, array
// (slice), or object (map).
//
// For slices and maps to work correctly, their element type should be interface{}. Since slices
// and maps are passed by reference, it is important that you not modify any elements of the slice
// or map after passing it to Custom, because the SDK might simultaneously be trying to access it
// on another goroutine.
func (b *userBuilderImpl) Custom(name string, value interface{}) UserBuilderCanMakeAttributePrivate {
	if b.custom == nil {
		b.custom = make(map[string]interface{})
	}
	b.custom[name] = value
	return b.canMakeAttributePrivate(name)
}

// Build creates a User from the current UserBuilder properties.
//
// The User is independent of the UserBuilder once you have called Build(); modifying the UserBuilder
// will not affect an already-created User.
func (b *userBuilderImpl) Build() User {
	key := b.key
	u := User{
		Key:       &key,
		Secondary: b.secondary.AsPointer(),
		Ip:        b.ip.AsPointer(),
		Country:   b.country.AsPointer(),
		Email:     b.email.AsPointer(),
		FirstName: b.firstName.AsPointer(),
		LastName:  b.lastName.AsPointer(),
		Avatar:    b.avatar.AsPointer(),
		Name:      b.name.AsPointer(),
	}
	if b.hasAnonymous {
		value := b.anonymous
		u.Anonymous = &value
	}
	if len(b.custom) > 0 {
		c := make(map[string]interface{}, len(b.custom))
		for k, v := range b.custom {
			c[k] = v
		}
		u.Custom = &c
	}
	if len(b.privateAttrs) > 0 {
		a := make([]string, 0, len(b.privateAttrs))
		for key, value := range b.privateAttrs {
			if value {
				a = append(a, key)
			}
		}
		u.PrivateAttributeNames = a
	}
	return u
}

// Marks the last attribute that was set on this builder as being a private attribute: that is, its value will not be
// sent to LaunchDarkly.
//
// This action only affects analytics events that are generated by this particular user object. To mark some (or all)
// user attributes as private for all users, use the Config properties PrivateAttributeName and AllAttributesPrivate.
//
// Most attributes can be made private, but Key and Anonymous cannot. This is enforced by the compiler, since the builder
// methods for attributes that can be made private are the only ones that return UserBuilderCanMakeAttributePrivate;
// therefore, you cannot write an expression like NewUserBuilder("user-key").AsPrivateAttribute().
//
// In this example, FirstName and LastName are marked as private, but Country is not:
//
//     user := NewUserBuilder("user-key").
//         FirstName("Pierre").AsPrivateAttribute().
//         LastName("Menard").AsPrivateAttribute().
//         Country("ES").
//         Build()
func (b *userBuilderCanMakeAttributePrivate) AsPrivateAttribute() UserBuilder {
	if b.builder.privateAttrs == nil {
		b.builder.privateAttrs = make(map[string]bool)
	}
	b.builder.privateAttrs[b.attrName] = true
	return b.builder
}

// Key changes the unique key for the user being built.
func (b *userBuilderCanMakeAttributePrivate) Key(value string) UserBuilder {
	return b.builder.Key(value)
}

// Secondary sets the secondary key attribute for the user being built.
//
// This affects feature flag targeting (https://docs.launchdarkly.com/docs/targeting-users#section-targeting-rules-based-on-user-attributes)
// as follows: if you have chosen to bucket users by a specific attribute, the secondary key (if set)
// is used to further distinguish between users who are otherwise identical according to that attribute.
func (b *userBuilderCanMakeAttributePrivate) Secondary(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.Secondary(value)
}

// IP sets the IP address attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) IP(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.IP(value)
}

// Country sets the country attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) Country(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.Country(value)
}

// Email sets the email address attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) Email(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.Email(value)
}

// FirstName sets the first name attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) FirstName(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.FirstName(value)
}

// LastName sets the last name attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) LastName(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.LastName(value)
}

// Avatar sets the avatar URL attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) Avatar(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.Avatar(value)
}

// Name sets the full name attribute for the user being built.
func (b *userBuilderCanMakeAttributePrivate) Name(value string) UserBuilderCanMakeAttributePrivate {
	return b.builder.Name(value)
}

// Anonymous sets the anonymous attribute for the user being built.
//
// If a user is anonymous, the user key will not appear on your LaunchDarkly dashboard.
func (b *userBuilderCanMakeAttributePrivate) Anonymous(value bool) UserBuilder {
	return b.builder.Anonymous(value)
}

// Custom sets a custom attribute for the user being built.
//
// A custom attribute value can be of any type supported by JSON: boolean, number, string, array
// (slice), or object (map).
//
// For slices and maps to work correctly, their element type should be interface{}. Since slices
// and maps are passed by reference, it is important that you not modify any elements of the slice
// or map after passing it to Custom, because the SDK might simultaneously be trying to access it
// on another goroutine.
func (b *userBuilderCanMakeAttributePrivate) Custom(name string, value interface{}) UserBuilderCanMakeAttributePrivate {
	return b.builder.Custom(name, value)
}

// Build creates a User from the current UserBuilder properties.
//
// The User is independent of the UserBuilder once you have called Build(); modifying the UserBuilder
// will not affect an already-created User.
func (b *userBuilderCanMakeAttributePrivate) Build() User {
	return b.builder.Build()
}
