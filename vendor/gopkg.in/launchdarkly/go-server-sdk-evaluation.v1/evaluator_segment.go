package evaluation

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

// SegmentExplanation describes a rule that determines whether a user was included in or excluded from a segment
type SegmentExplanation struct {
	Kind        string
	MatchedRule *ldmodel.SegmentRule
}

func segmentContainsUser(s ldmodel.Segment, user *lduser.User) (bool, SegmentExplanation) {
	userKey := user.GetKey()

	// Check if the user is included in the segment by key
	for _, key := range s.Included {
		if userKey == key {
			return true, SegmentExplanation{Kind: "included"}
		}
	}

	// Check if the user is excluded from the segment by key
	for _, key := range s.Excluded {
		if userKey == key {
			return false, SegmentExplanation{Kind: "excluded"}
		}
	}

	// Check if any of the segment rules match
	for _, rule := range s.Rules {
		if segmentRuleMatchesUser(rule, user, s.Key, s.Salt) {
			reason := rule
			return true, SegmentExplanation{Kind: "rule", MatchedRule: &reason}
		}
	}

	return false, SegmentExplanation{}
}

func segmentRuleMatchesUser(r ldmodel.SegmentRule, user *lduser.User, key, salt string) bool {
	for _, clause := range r.Clauses {
		c := clause
		if !clauseMatchesUserNoSegments(&c, user) {
			return false
		}
	}

	// If the Weight is absent, this rule matches
	if r.Weight == nil {
		return true
	}

	// All of the clauses are met. Check to see if the user buckets in
	bucketBy := lduser.KeyAttribute
	if r.BucketBy != nil {
		bucketBy = *r.BucketBy
	}

	// Check whether the user buckets into the segment
	bucket := bucketUser(user, key, bucketBy, salt)
	weight := float32(*r.Weight) / 100000.0

	return bucket < weight
}
