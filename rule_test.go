package ldclient

import (
	"testing"
)

/*

Tests rules (conjunctions of clauses)

*/

// [email in {gmail.com, hotmail.com}] && [group in {Microsoft, Google}]
var hotmailOrGmailAndMsOrGoogleRule = Rule{
	Clauses:   []Clause{hotmailOrGmailClause, msOrGoogleClause},
	Variation: nil,
}

// [email in {gmail.com, hotmail.com}] && [not(group in {Youtube, Nest})]
var hotmailOrGmailAndNotYoutubeOrNest = Rule{
	Clauses: []Clause{hotmailOrGmailClause, notYoutubeOrNestClause},
}

func TestGoogleGroupAndEmailRule(t *testing.T) {
	if !hotmailOrGmailAndMsOrGoogleRule.matchesUser(googleEmployee) {
		t.Error("Expected Google employee to match group and e-mail rule")
	}
}

func TestGoogleEmailButNotYoutubeGroup(t *testing.T) {
	if hotmailOrGmailAndNotYoutubeOrNest.matchesUser(googleEmployee) {
		t.Errorf("Google employee should not match rule (YouTube group should be excluded)")
	}
}
