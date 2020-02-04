package ldclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v1/ldvalue"
)

func TestScrubUser(t *testing.T) {
	t.Run("private built-in attributes per user", func(t *testing.T) {
		filter := newUserFilter(DefaultConfig)
		user := NewUserBuilder("user-key").
			FirstName("sam").
			LastName("smith").
			Name("sammy").
			Country("freedonia").
			Avatar("my-avatar").
			IP("123.456.789").
			Email("me@example.com").
			Secondary("abcdef").
			Build()

		for _, attr := range BuiltinAttributes {
			user.PrivateAttributeNames = []string{attr}
			scrubbedUser := *filter.scrubUser(user)
			assert.Equal(t, []string{attr}, scrubbedUser.PrivateAttributes)
			scrubbedUser.PrivateAttributes = nil
			assert.NotEqual(t, user, scrubbedUser)
		}
	})

	t.Run("global private built-in attributes", func(t *testing.T) {
		user := NewUserBuilder("user-key").
			FirstName("sam").
			LastName("smith").
			Name("sammy").
			Country("freedonia").
			Avatar("my-avatar").
			IP("123.456.789").
			Email("me@example.com").
			Secondary("abcdef").
			Build()

		for _, attr := range BuiltinAttributes {
			filter := newUserFilter(Config{PrivateAttributeNames: []string{attr}})
			scrubbedUser := *filter.scrubUser(user)
			assert.Equal(t, []string{attr}, scrubbedUser.PrivateAttributes)
			scrubbedUser.PrivateAttributes = nil
			assert.NotEqual(t, user, scrubbedUser)
		}
	})

	t.Run("private custom attribute", func(t *testing.T) {
		filter := newUserFilter(DefaultConfig)
		userKey := "userKey"
		user := NewUserBuilder(userKey).
			Custom("my-secret-attr", ldvalue.String("my secret value")).AsPrivateAttribute().
			Custom("non-secret-attr", ldvalue.String("OK value")).
			Build()

		scrubbedUser := *filter.scrubUser(user)

		assert.Equal(t, []string{"my-secret-attr"}, scrubbedUser.PrivateAttributes)
		assert.NotContains(t, *scrubbedUser.Custom, "my-secret-attr")
	})

	t.Run("all attributes private", func(t *testing.T) {
		filter := newUserFilter(Config{AllAttributesPrivate: true})
		userKey := "userKey"
		user := NewUserBuilder(userKey).
			FirstName("sam").
			LastName("smith").
			Name("sammy").
			Country("freedonia").
			Avatar("my-avatar").
			IP("123.456.789").
			Email("me@example.com").
			Secondary("abcdef").
			Custom("my-secret-attr", ldvalue.String("my-secret-value")).
			Build()

		scrubbedUser := *filter.scrubUser(user)
		sort.Strings(scrubbedUser.PrivateAttributes)
		expectedAttributes := append(BuiltinAttributes, "my-secret-attr")
		sort.Strings(expectedAttributes)
		assert.Equal(t, expectedAttributes, scrubbedUser.PrivateAttributes)

		scrubbedUser.PrivateAttributes = nil
		assert.Equal(t, NewUser(userKey), scrubbedUser.User)
	})

	t.Run("anonymous attribute can't be private", func(t *testing.T) {
		filter := newUserFilter(Config{AllAttributesPrivate: true})
		user := NewUserBuilder(userKey).Anonymous(true).Build()

		scrubbedUser := *filter.scrubUser(user)
		assert.Equal(t, user, scrubbedUser.User)
	})
}

func TestUserSerialization(t *testing.T) {
	var errorMessage = "don't serialize me, bro"

	doUserSerializationErrorTest := func(errorValue interface{}, withUserKey bool) {
		logger := newMockLogger("")
		config := DefaultConfig
		config.Loggers.SetBaseLogger(logger)
		config.LogUserKeyInErrors = withUserKey
		filter := newUserFilter(config)
		user := NewUserBuilder("user-key").
			FirstName("sam").
			Email("test@example.com").
			Build()

		// To inject our problematic value, we need to access the Custom map directly. In a future version
		// this will no longer be possible.
		custom := make(map[string]interface{})
		custom["problem"] = errorValue
		user.Custom = &custom

		scrubbedUser := filter.scrubUser(user)
		bytes, err := json.Marshal(scrubbedUser)
		assert.NoError(t, err)

		expectedMessage := "ERROR: " + fmt.Sprintf(userSerializationErrorMessage, describeUserForErrorLog(&user, withUserKey), errorMessage)
		assert.Equal(t, []string{expectedMessage}, logger.output)

		// Verify that we did marshal all of the user attributes except the custom ones
		expectedUser := user
		expectedUser.Custom = nil
		resultUser := NewUser("")
		err = json.Unmarshal(bytes, &resultUser)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, resultUser)
	}

	t.Run("error in serialization of custom attributes is caught", func(t *testing.T) {
		doUserSerializationErrorTest(valueThatErrorsWhenMarshalledToJSON(errorMessage), false)
	})

	t.Run("panic in serialization of custom attributes is caught", func(t *testing.T) {
		doUserSerializationErrorTest(valueThatPanicsWhenMarshalledToJSON(errorMessage), false)
	})

	t.Run("error message includes user key depending on configuration", func(t *testing.T) {
		doUserSerializationErrorTest(valueThatErrorsWhenMarshalledToJSON(errorMessage), true)
	})

	t.Run("panic message includes user key depending on configuration", func(t *testing.T) {
		doUserSerializationErrorTest(valueThatPanicsWhenMarshalledToJSON(errorMessage), true)
	})
}

func strPtr(s string) *string {
	return &s
}

type valueThatErrorsWhenMarshalledToJSON string
type valueThatPanicsWhenMarshalledToJSON string

func (v valueThatErrorsWhenMarshalledToJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New(string(v))
}

func (v valueThatPanicsWhenMarshalledToJSON) MarshalJSON() ([]byte, error) {
	panic(errors.New(string(v)))
}
