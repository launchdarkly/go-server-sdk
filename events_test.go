package ldclient

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

var BuiltinAttributes = []string{
	"avatar",
	"country",
	"email",
	"firstName",
	"ip",
	"lastName",
	"name",
	"secondary",
}

func init() {
	sort.Strings(BuiltinAttributes)
}

func TestScrubUser(t *testing.T) {
	t.Run("private built-in attributes", func(t *testing.T) {
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
			scrubbedUser := scrubUser(user, false)
			assert.Equal(t, []string{attr}, scrubbedUser.PrivateAttributes)
			scrubbedUser.PrivateAttributes = nil
			assert.NotEqual(t, user, scrubbedUser)
		}
	})

	t.Run("private custom attribute", func(t *testing.T) {
		userKey := "userKey"
		user := User{
			Key: &userKey,
			PrivateAttributeNames: []string{"my-secret-attr"},
			Custom: &map[string]interface{}{
				"my-secret-attr": "my secret value",
			}}

		scrubbedUser := scrubUser(user, false)

		assert.Equal(t, []string{"my-secret-attr"}, scrubbedUser.PrivateAttributes)
		assert.NotContains(t, *scrubbedUser.Custom, "my-secret-attr")
	})

	t.Run("all attributes private", func(t *testing.T) {
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

		scrubbedUser := scrubUser(user, true)
		sort.Strings(scrubbedUser.PrivateAttributes)
		expectedAttributes := append(BuiltinAttributes, "my-secret-attr")
		sort.Strings(expectedAttributes)
		assert.Equal(t, expectedAttributes, scrubbedUser.PrivateAttributes)

		scrubbedUser.PrivateAttributes = nil
		assert.Equal(t, User{Key: &userKey, Custom: &map[string]interface{}{}}, scrubbedUser)
		assert.NotContains(t, *scrubbedUser.Custom, "my-secret-attr")
		assert.Nil(t, scrubbedUser.Name)
	})
}

func strPtr(s string) *string {
	return &s
}
