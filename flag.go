package ldclient

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"reflect"
	"strconv"
)

type FeatureFlag struct {
	Name         string      `json:"name" bson:"name"`
	Key          string      `json:"key" bson:"key"`
	Version      int         `json:"version" bson:"version"`
	On           bool        `json:"on" bson:"on"`
	Salt         string      `json:"salt" bson:"salt"`
	Sel          string      `json:"sel" bson:"sel"`
	Targets      []Target    `json:"targets" bson:"targets"`
	Rules        []Rule      `json:"rules" bson:"rules"`
	Fallthrough  Rule        `json:"fallthrough" bson:"fallthrough"`
	OffVariation interface{} `json:"offVariation" bson:"offVariation"`
	Archived     bool        `json:"archived" bson:"archived"`
}

type Rule struct {
	Clauses   []Clause            `json:"clauses,omitempty" bson:"clauses"`
	Variation interface{}         `json:"variation,omitempty" bson:"variation"`
	Rollout   []WeightedVariation `json:"rollout,omitempty" bson:"rollout"`
}

type Clause struct {
	Attribute string        `json:"attribute" bson:"attribute"`
	Op        Operator      `json:"op" bson:"op"`
	Values    []interface{} `json:"values" bson:"values"` // An array, interpreted as an OR of values
	Negate    bool          `json:"negate" bson:"negate"`
}

type WeightedVariation struct {
	Variation interface{} `json:"variation" bson:"variation"`
	Weight    int         `json:"weight" bson:"weight"` // Ranges from 0 to 100000
}

type Target struct {
	Values    []string    `json:"values" bson:"values"`
	Variation interface{} `json:"variation" bson:"variation"`
}

// An explanation is either a target or a rule
type Explanation struct {
	Kind string `json:"kind" bson:"kind"`
	*Target
	*Rule
}

func (f FeatureFlag) bucketUser(user User) float32 {
	var idHash string

	// Precondition: we've already checked that the key is non-nil
	idHash = *user.Key

	if user.Secondary != nil {
		idHash = idHash + "." + *user.Secondary
	}

	h := sha1.New()
	io.WriteString(h, f.Key+"."+f.Salt+"."+idHash)
	hash := hex.EncodeToString(h.Sum(nil))[:15]

	intVal, _ := strconv.ParseInt(hash, 16, 64)

	bucket := float32(intVal) / long_scale

	return bucket
}

func (f FeatureFlag) EvaluateExplain(user User) (interface{}, *Explanation) {
	// TODO: The toggle algorithm should check the kill switch. We won't check it here
	// so we can potentially compute an explanation even if the kill switch is hit
	if user.Key == nil {
		return f.OffVariation, nil
	}

	for _, target := range f.Targets {
		for _, value := range target.Values {
			if value == *user.Key {
				explanation := Explanation{Kind: "target", Target: &target}
				return target.Variation, &explanation
			}
		}
	}

	bucket := f.bucketUser(user)

	for _, rule := range f.Rules {
		if rule.matchesUser(user) {
			variation, passErr := rule.valueForUser(user, bucket)

			if passErr {
				return f.OffVariation, nil
			} else {
				explanation := Explanation{Kind: "rule", Rule: &rule}
				return variation, &explanation
			}
		}
	}

	return f.OffVariation, nil
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

func (r Rule) valueForUser(user User, bucket float32) (interface{}, bool) {
	if r.Variation != nil {
		return r.Variation, false
	}

	var sum float32 = 0.0

	for _, wv := range r.Rollout {
		sum += float32(wv.Weight) / 100000.0
		if bucket < sum {
			return wv.Variation, false
		}
	}

	return nil, true
}
