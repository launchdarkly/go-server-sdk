package ldclient

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"reflect"
	"strconv"
)

type FeatureFlag struct {
	Name         string      `json:"name"`
	Key          string      `json:"key"`
	Version      int         `json:"version"`
	On           bool        `json:"on"`
	Salt         string      `json:"salt"`
	Sel          string      `json:"sel"`
	Conditions   []Rule      `json:"conditions"`
	Fallthrough  Rule        `json:"fallthrough"`
	OffVariation interface{} `json:"offVariation"`
}

type Clause struct {
	Attribute string        `json:"attribute"`
	Op        Operator      `json:"operator"`
	Values    []interface{} `json:"values"` // An array, interpreted as an OR of values
	Negate    bool          `json:"negate"`
}

type WeightedVariation struct {
	Variation interface{} `json:"variation"`
	Weight    int         `json:"weight"` // Ranges from 0 to 100000
}

type Rule struct {
	Conditions []Clause            `json:"conditions,omitempty"`
	Variation  interface{}         `json:"variation,omitempty"`
	Rollout    []WeightedVariation `json:"rollout,omitempty"`
}

func (f FeatureFlag) bucketUser(user User) (float32, bool) {
	var idHash string

	if user.Key != nil {
		idHash = *user.Key
	} else { // without a key, this rule should pass
		return 0, true
	}

	if user.Secondary != nil {
		idHash = idHash + "." + *user.Secondary
	}

	h := sha1.New()
	io.WriteString(h, f.Key+"."+f.Salt+"."+idHash)
	hash := hex.EncodeToString(h.Sum(nil))[:15]

	intVal, _ := strconv.ParseInt(hash, 16, 64)

	bucket := float32(intVal) / long_scale

	return bucket, false
}

func (f FeatureFlag) EvaluateExplain(user User) (interface{}, *Rule, bool) {
	if !f.On {
		return f.OffVariation, nil, true
	}

	bucket, passErr := f.bucketUser(user)

	if passErr {
		return f.OffVariation, nil, true
	}

	for _, rule := range f.Conditions {
		if rule.matchesUser(user) {
			variation, passErr := rule.valueForUser(user, bucket)

			if passErr {
				return f.OffVariation, nil, true
			} else {
				return variation, &rule, false
			}
		}
	}

	return f.OffVariation, nil, true
}

func (r Rule) matchesUser(user User) bool {
	for _, clause := range r.Conditions {
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
