package ldclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubUser(t *testing.T) {
	t.Run("private built-in attributes per user", func(t *testing.T) {
		filter := newUserFilter(DefaultConfig)
		user := User{
			Key:       strPtr("user-key"),
			FirstName: strPtr("sam"),
			LastName:  strPtr("smith"),
			Name:      strPtr("sammy"),
			Country:   strPtr("freedonia"),
			Avatar:    strPtr("my-avatar"),
			Ip:        strPtr("123.456.789"),
			Email:     strPtr("me@example.com"),
			Secondary: strPtr("abcdef"),
		}

		for _, attr := range BuiltinAttributes {
			user.PrivateAttributeNames = []string{attr}
			scrubbedUser := *filter.scrubUser(user)
			assert.Equal(t, []string{attr}, scrubbedUser.PrivateAttributes)
			scrubbedUser.PrivateAttributes = nil
			assert.NotEqual(t, user, scrubbedUser)
		}
	})

	t.Run("global private built-in attributes", func(t *testing.T) {
		user := User{
			Key:       strPtr("user-key"),
			FirstName: strPtr("sam"),
			LastName:  strPtr("smith"),
			Name:      strPtr("sammy"),
			Country:   strPtr("freedonia"),
			Avatar:    strPtr("my-avatar"),
			Ip:        strPtr("123.456.789"),
			Email:     strPtr("me@example.com"),
			Secondary: strPtr("abcdef"),
		}

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
		user := User{
			Key: &userKey,
			PrivateAttributeNames: []string{"my-secret-attr"},
			Custom: &map[string]interface{}{
				"my-secret-attr":  "my secret value",
				"non-secret-attr": "OK value",
			}}

		scrubbedUser := *filter.scrubUser(user)

		assert.Equal(t, []string{"my-secret-attr"}, scrubbedUser.PrivateAttributes)
		assert.NotContains(t, *scrubbedUser.Custom, "my-secret-attr")
	})

	t.Run("all attributes private", func(t *testing.T) {
		filter := newUserFilter(Config{AllAttributesPrivate: true})
		userKey := "userKey"
		user := User{
			Key:       &userKey,
			FirstName: strPtr("sam"),
			LastName:  strPtr("smith"),
			Name:      strPtr("sammy"),
			Country:   strPtr("freedonia"),
			Avatar:    strPtr("my-avatar"),
			Ip:        strPtr("123.456.789"),
			Email:     strPtr("me@example.com"),
			Secondary: strPtr("abcdef"),
			Custom: &map[string]interface{}{
				"my-secret-attr": "my secret value",
			}}

		scrubbedUser := *filter.scrubUser(user)
		sort.Strings(scrubbedUser.PrivateAttributes)
		expectedAttributes := append(BuiltinAttributes, "my-secret-attr")
		sort.Strings(expectedAttributes)
		assert.Equal(t, expectedAttributes, scrubbedUser.PrivateAttributes)

		scrubbedUser.PrivateAttributes = nil
		assert.Equal(t, User{Key: &userKey}, scrubbedUser.User)
	})

	t.Run("anonymous attribute can't be private", func(t *testing.T) {
		filter := newUserFilter(Config{AllAttributesPrivate: true})
		userKey := "userKey"
		anon := true
		user := User{
			Key:       &userKey,
			Anonymous: &anon}

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
		user := User{
			Key:       strPtr("user-key"),
			FirstName: strPtr("sam"),
			Email:     strPtr("test@example.com"),
		}
		custom := map[string]interface{}{"problem": errorValue}
		user.Custom = &custom

		scrubbedUser := filter.scrubUser(user)
		bytes, err := json.Marshal(scrubbedUser)
		assert.NoError(t, err)

		expectedMessage := "ERROR: " + fmt.Sprintf(userSerializationErrorMessage, describeUserForErrorLog(&user, withUserKey), errorMessage)
		assert.Equal(t, []string{expectedMessage}, logger.output)

		// Verify that we did marshal all of the user attributes except the custom ones
		expectedUser := user
		expectedUser.Custom = nil
		resultUser := User{}
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
