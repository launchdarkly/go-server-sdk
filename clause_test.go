package ldclient

import (
	"testing"
)

var (
	key            = "sample-key"
	gmailAddress   = "foo@gmail.com"
	hotmailAddress = "bar@hotmail.com"
)

var yammerGroups interface{} = []string{"Yammer", "Microsoft"}
var youtubeGroups interface{} = []string{"Youtube", "Google"}

var yammerCustom = map[string]interface{}{"group": yammerGroups}
var youtubeCustom = map[string]interface{}{"group": youtubeGroups}

// Matches users with gmail.com or hotmail.com e-mail addresses
var emailClause = Clause{
	Attribute: "email",
	Op:        operatorEndsWith,
	Values:    []interface{}{"gmail.com", "hotmail.com"},
	Negate:    false,
}

// Matches users in the Microsoft or Google groups
var groupClause = Clause{
	Attribute: "group",
	Op:        operatorIn,
	Values:    []interface{}{"Microsoft", "Google"},
	Negate:    false,
}

var msEmployee = User{
	Key:    &key,
	Email:  &hotmailAddress,
	Custom: &yammerCustom,
}

var googleEmployee = User{
	Key:    &key,
	Email:  &gmailAddress,
	Custom: &youtubeCustom,
}

func TestEmailEndsWithMatches(t *testing.T) {
	if !emailClause.matchesUser(msEmployee) {
		t.Error("Expecting MS employee to match email rule")
	}
}

func TestGroupMatches(t *testing.T) {
	if !groupClause.matchesUser(googleEmployee) {
		t.Error("Expecting Google employee to match groups rule")
	}
}
