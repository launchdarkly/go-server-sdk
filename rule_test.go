package ldclient

import (
	"testing"
)

// [email in {gmail.com, hotmail.com}] && [group in {Microsoft, Google}]
var hotmailOrGmailAndMsOrGoogleRule = Rule{
	Conditions: []Clause{hotmailOrGmailClause, msOrGoogleClause},
	Variation:  true,
}

// [email in {gmail.com, hotmail.com}] && [not(group in {Youtube, Bazzle})]
var hotmailOrGmailAndNotYoutubeOrBazzle = Rule{
	Conditions: []Clause{hotmailOrGmailClause, notYoutubeOrNest},
}

func TestGoogleGroupAndEmailRule(t *testing.T) {
	if !hotmailOrGmailAndMsOrGoogleRule.matchesUser(googleEmployee) {
		t.Error("Expected Google employee to match group and e-mail rule")
	}
}

func TestGoogleEmailButNotYoutubeGroup(t *testing.T) {
	if hotmailOrGmailAndNotYoutubeOrBazzle.matchesUser(googleEmployee) {
		t.Errorf("Google employee should not match rule (YouTube group should be excluded)")
	}
}
