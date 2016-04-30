package ldclient

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

const (
	long_scale = float32(0xFFFFFFFFFFFFFFF)
)

type FeatureFlag struct {
	Key           string         `json:"key" bson:"key"`
	Version       int            `json:"version" bson:"version"`
	On            bool           `json:"on" bson:"on"`
	Prerequisites []Prerequisite `json:"prerequisites,omitempty" bson:"prerequisites"`
	Salt          string         `json:"salt" bson:"salt"`
	Sel           string         `json:"sel" bson:"sel"`
	Targets       []Target       `json:"targets" bson:"targets"`
	Rules         []Rule         `json:"rules" bson:"rules"`
	Fallthrough   Rule           `json:"fallthrough" bson:"fallthrough"`
	OffVariation  *int           `json:"offVariation" bson:"offVariation"`
	Variations    []interface{}  `json:"variations" bson:"variations"`
	Deleted       bool           `json:"deleted,omitempty" bson:"deleted"`
}

// Expresses a set of AND-ed matching conditions for a user, along with
// either the fixed variation or percent rollout to serve if the conditions
// match.
// Invariant: one of the variation or rollout must be non-nil.
type Rule struct {
	Clauses   []Clause `json:"clauses" bson:"clauses"`
	Variation *int     `json:"variation,omitempty" bson:"variation,omitempty"`
	Rollout   *Rollout `json:"rollout,omitempty" bson:"rollout,omitempty"`
}

type Rollout struct {
	Variations []WeightedVariation `json:"variations" bson:"variations"`
	BucketBy   *string             `json:"bucketBy,omitempty" bson:"bucketBy,omitempty"`
}

type Clause struct {
	Attribute string        `json:"attribute" bson:"attribute"`
	Op        Operator      `json:"op" bson:"op"`
	Values    []interface{} `json:"values" bson:"values"` // An array, interpreted as an OR of values
	Negate    bool          `json:"negate" bson:"negate"`
}

type WeightedVariation struct {
	Variation int `json:"variation" bson:"variation"`
	Weight    int `json:"weight" bson:"weight"` // Ranges from 0 to 100000
}

type Target struct {
	Values    []string `json:"values" bson:"values"`
	Variation int      `json:"variation" bson:"variation"`
}

// An explanation is either a target or a rule or a prerequisite that wasn't met
type Explanation struct {
	Kind          string `json:"kind" bson:"kind"`
	*Target       `json:"target,omitempty"`
	*Rule         `json:"rule,omitempty"`
	*Prerequisite `json:"prerequisite,omitempty"`
}

type Prerequisite struct {
	Key       string `json:"key"`
	Variation int    `json:"variation"`
}

func bucketUser(user User, key, attr, salt string) float32 {
	uValue, pass := user.valueOf(attr)

	if idHash, ok := uValue.(string); pass || !ok {
		return 0
	} else {
		if user.Secondary != nil {
			idHash = idHash + "." + *user.Secondary
		}

		h := sha1.New()
		io.WriteString(h, key+"."+salt+"."+idHash)
		hash := hex.EncodeToString(h.Sum(nil))[:15]

		intVal, _ := strconv.ParseInt(hash, 16, 64)

		bucket := float32(intVal) / long_scale

		return bucket
	}
}

type EvalResult struct {
	Value                interface{}
	Explanation          *Explanation
	FeatureRequestEvents []FeatureRequestEvent //to be sent to LD
}

//struct only used in this file to allow for cycle detection in prereqs.
type evalResultInternal struct {
	EvalResult
	visitedFeatureKeys map[string]bool
}

func (f FeatureFlag) EvaluateExplain(user User, store FeatureStore) (*EvalResult, error) {
	if user.Key == nil {
		return nil, nil
	}
	events := make([]FeatureRequestEvent, 1)
	visited := make(map[string]bool)
	evalResultInternal, err := f.evaluateExplain(user, store, events, visited)
	if evalResultInternal != nil {
		return &evalResultInternal.EvalResult, err
	}
	return nil, err
}

func (f FeatureFlag) evaluateExplain(user User, store FeatureStore, events []FeatureRequestEvent, visited map[string]bool) (*evalResultInternal, error) {
	var failedPrereq Prerequisite
	prereqsOk := true
	for _, prereq := range f.Prerequisites {
		visited[f.Key] = true
		if _, ok := visited[prereq.Key]; ok {
			//TODO: ok to skip sending actual EvalResult in these error cases?
			return nil, fmt.Errorf("Cycle detected in prerequisites when evaluating feature key: %s", prereq.Key)
		}
		prereqFeatureFlag, err := store.Get(prereq.Key)
		if err != nil {
			return nil, err
		}
		if prereqFeatureFlag == nil {
			return nil, fmt.Errorf("Prerequisite feature flag not found: %+v", prereq.Key)
		}
		if !prereqFeatureFlag.On {
			//TODO: use offVariation or set prereqsOk to false?
			//TODO: Should we keep recursing prereqs?
		}

		prereqEvalResult, err := prereqFeatureFlag.evaluateExplain(user, store, events, visited)
		if err != nil {
			return nil, err
		}
		visited = prereqEvalResult.visitedFeatureKeys
		events = prereqEvalResult.FeatureRequestEvents
		events = append(events, NewFeatureRequestEvent(prereq.Key, user, prereqEvalResult.Value, nil))
		if prereqEvalResult.Value != prereqFeatureFlag.getVariation(&prereq.Variation) {
			failedPrereq = prereq
			prereqsOk = false
		}
	}
	if !prereqsOk {
		explanation := Explanation{
			Kind:         "prerequisite",
			Prerequisite: &failedPrereq} //return the last prereq to fail

		return &evalResultInternal{EvalResult{nil, &explanation, events}, visited}, nil
	}

	index, explanation := f.evaluateExplainIndex(user)
	return &evalResultInternal{EvalResult{f.getVariation(index), explanation, events}, visited}, nil
}

func (f FeatureFlag) getVariation(index *int) interface{} {
	if index == nil || *index >= len(f.Variations) {
		return nil
	} else {
		return f.Variations[*index]
	}
}

func (f FeatureFlag) evaluateExplainIndex(user User) (*int, *Explanation) {
	// Check to see if targets match
	for _, target := range f.Targets {
		for _, value := range target.Values {
			if value == *user.Key {
				explanation := Explanation{Kind: "target", Target: &target}
				return &target.Variation, &explanation
			}
		}
	}

	// Now walk through the rules and see if any match
	for _, rule := range f.Rules {
		if rule.matchesUser(user) {
			variation := rule.variationIndexForUser(user, f.Key, f.Salt)

			if variation == nil {
				return nil, nil
			} else {
				explanation := Explanation{Kind: "rule", Rule: &rule}
				return variation, &explanation
			}
		}
	}

	// Walk through the fallthrough and see if it matches
	variation := f.Fallthrough.variationIndexForUser(user, f.Key, f.Salt)

	if variation == nil {
		return nil, nil
	} else {
		explanation := Explanation{Kind: "fallthrough", Rule: &f.Fallthrough}
		return variation, &explanation
	}
}

func (r Rule) matchesUser(user User) bool {
	for _, clause := range r.Clauses {
		if !clause.matchesUser(user) {
			return false
		}
	}
	return true
}

func (c Clause) matchesUser(user User) bool {
	uValue, pass := user.valueOf(c.Attribute)

	if pass {
		return false
	}
	matchFn := operatorFn(c.Op)

	val := reflect.ValueOf(uValue)

	// If the user value is an array or slice,
	// see if the intersection is non-empty. If so,
	// this clause matches
	if val.Kind() == reflect.Array || val.Kind() == reflect.Slice {
		for i := 0; i < val.Len(); i++ {
			if matchAny(matchFn, val.Index(i).Interface(), c.Values) {
				return c.maybeNegate(true)
			}
		}
		return c.maybeNegate(false)
	}

	return c.maybeNegate(matchAny(matchFn, uValue, c.Values))
}

func (c Clause) maybeNegate(b bool) bool {
	if c.Negate {
		return !b
	} else {
		return b
	}
}

func matchAny(fn opFn, value interface{}, values []interface{}) bool {
	for _, v := range values {
		if fn(value, v) {
			return true
		}
	}
	return false
}

func (r Rule) variationIndexForUser(user User, key, salt string) *int {
	if r.Variation != nil {
		return r.Variation
	} else if r.Rollout != nil {
		bucketBy := "key"
		if r.Rollout.BucketBy != nil {
			bucketBy = *r.Rollout.BucketBy
		}

		var bucket = bucketUser(user, key, bucketBy, salt)
		var sum float32 = 0.0

		for _, wv := range r.Rollout.Variations {
			sum += float32(wv.Weight) / 100000.0
			if bucket < sum {
				return &wv.Variation
			}
		}
	}
	return nil
}
