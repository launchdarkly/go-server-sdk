package ldclient

import (
	"testing"
)

/*

Tests individual clauses

*/

var (
	key              = "sample-key"
	gmailAddress     = "foo@gmail.com"
	microsoftAddress = "bar@microsoft.com"
)

var yammerGroups interface{} = []string{"Yammer", "Microsoft"}
var youtubeGroups interface{} = []string{"Youtube", "Google"}

var yammerCustom = map[string]interface{}{"group": yammerGroups}
var youtubeCustom = map[string]interface{}{"group": youtubeGroups}

// email in {gmail.com, hotmail.com}
var hotmailOrGmailClause = Clause{
	Attribute: "email",
	Op:        operatorEndsWith,
	Values:    []interface{}{"gmail.com", "hotmail.com"},
	Negate:    false,
}

// group in {Microsoft, Google}
var msOrGoogleClause = Clause{
	Attribute: "group",
	Op:        operatorIn,
	Values:    []interface{}{"Microsoft", "Google"},
	Negate:    false,
}

// not(group in {Youtube, Nest})
var notYoutubeOrNestClause = Clause{
	Attribute: "group",
	Op:        operatorIn,
	Values:    []interface{}{"Youtube", "Nest"},
	Negate:    true,
}

var msEmployee = User{
	Key:    &key,
	Email:  &microsoftAddress,
	Custom: &yammerCustom,
}

var googleEmployee = User{
	Key:    &key,
	Email:  &gmailAddress,
	Custom: &youtubeCustom,
}

var emptyUser = User{
	Key: nil,
}

func TestEmptyUserNeverMatchesClause(t *testing.T) {
	if hotmailOrGmailClause.matchesUser(emptyUser) {
		t.Error("Empty user should not match any clauses")
	}
	if msOrGoogleClause.matchesUser(emptyUser) {
		t.Error("Empty user should not match any clauses")
	}
	if notYoutubeOrNestClause.matchesUser(emptyUser) {
		t.Error("Empty user should not match any clauses")
	}
}

func TestHotmailOrGmailEmail(t *testing.T) {
	if !hotmailOrGmailClause.matchesUser(googleEmployee) {
		t.Error("Expecting Google employee to match email rule")
	}
}

func TestMsOrGoogleGroup(t *testing.T) {
	if !msOrGoogleClause.matchesUser(googleEmployee) {
		t.Error("Expecting Google employee to match groups rule")
	}
}

func TestNotYoutubeOrNest(t *testing.T) {
	if !notYoutubeOrNestClause.matchesUser(msEmployee) {
		t.Error("Expecting Microsoft employee to match not Youtube rule")
	}
	if notYoutubeOrNestClause.matchesUser(googleEmployee) {
		t.Error("Expecting Google employee to not match Youtube rule")
	}
}
