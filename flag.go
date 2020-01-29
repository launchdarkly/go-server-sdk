package ldclient

import (
	"crypto/sha1" // nolint:gas // just used for insecure hashing
	"encoding/hex"
	"io"
	"math"
	"reflect"
	"strconv"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

const (
	longScale = float32(0xFFFFFFFFFFFFFFF)
	userKey   = "key"
)

// FeatureFlag describes an individual feature flag.
//
// Deprecated: this type is for internal use and will be moved to another package in a future version.
type FeatureFlag struct {
	Key                    string             `json:"key" bson:"key"`
	Version                int                `json:"version" bson:"version"`
	On                     bool               `json:"on" bson:"on"`
	TrackEvents            bool               `json:"trackEvents" bson:"trackEvents"`
	TrackEventsFallthrough bool               `json:"trackEventsFallthrough" bson:"trackEventsFallthrough"`
	Deleted                bool               `json:"deleted" bson:"deleted"`
	Prerequisites          []prerequisite     `json:"prerequisites" bson:"prerequisites"`
	Salt                   string             `json:"salt" bson:"salt"`
	Sel                    string             `json:"sel" bson:"sel"`
	Targets                []target           `json:"targets" bson:"targets"`
	Rules                  []flagRule         `json:"rules" bson:"rules"`
	Fallthrough            variationOrRollout `json:"fallthrough" bson:"fallthrough"`
	OffVariation           *int               `json:"offVariation" bson:"offVariation"`
	Variations             []interface{}      `json:"variations" bson:"variations"`
	DebugEventsUntilDate   *uint64            `json:"debugEventsUntilDate" bson:"debugEventsUntilDate"`
	ClientSide             bool               `json:"clientSide" bson:"-"`
}

// GetKey returns the string key for the feature flag
func (f *FeatureFlag) GetKey() string {
	return f.Key
}

// GetVersion returns the version of a flag
func (f *FeatureFlag) GetVersion() int {
	return f.Version
}

// IsDeleted returns whether a flag has been deleted
func (f *FeatureFlag) IsDeleted() bool {
	return f.Deleted
}

// Clone returns a copy of a flag
func (f *FeatureFlag) Clone() VersionedData {
	f1 := *f
	return &f1
}

// featureFlagVersionedDataKind implements VersionedDataKind and provides methods to build storage engine for flags.
type featureFlagVersionedDataKind struct{}

// GetNamespace returns the a unique namespace identifier for feature flag objects
func (fk featureFlagVersionedDataKind) GetNamespace() string {
	return "features"
}

// String returns the namespace
func (fk featureFlagVersionedDataKind) String() string {
	return fk.GetNamespace()
}

// GetDefaultItem returns a default feature flag representation
func (fk featureFlagVersionedDataKind) GetDefaultItem() interface{} {
	return &FeatureFlag{}
}

// MakeDeletedItem returns representation of a deleted flag
func (fk featureFlagVersionedDataKind) MakeDeletedItem(key string, version int) VersionedData {
	return &FeatureFlag{Key: key, Version: version, Deleted: true}
}

// Features is a convenience variable to access an instance of VersionedDataKind.
//
// Deprecated: this variable is for internal use and will be removed in a future version.
var Features VersionedDataKind = featureFlagVersionedDataKind{}

// flagRule expresses a set of AND-ed matching conditions for a user, along with either a fixed
// variation or a set of rollout percentages.
type flagRule struct {
	variationOrRollout `bson:",inline"`
	ID                 string   `json:"id,omitempty" bson:"id,omitempty"`
	Clauses            []clause `json:"clauses" bson:"clauses"`
	TrackEvents        bool     `json:"trackEvents" bson:"trackEvents"`
}

// variationOrRollout contains either the fixed variation or percent rollout to serve.
// Invariant: one of the variation or rollout must be non-nil.
type variationOrRollout struct {
	Variation *int     `json:"variation,omitempty" bson:"variation,omitempty"`
	Rollout   *rollout `json:"rollout,omitempty" bson:"rollout,omitempty"`
}

// rollout describes how users will be bucketed into variations during a percentage rollout.
type rollout struct {
	Variations []weightedVariation `json:"variations" bson:"variations"`
	BucketBy   *string             `json:"bucketBy,omitempty" bson:"bucketBy,omitempty"`
}

// clause describes an individual cluuse within a targeting rule.
type clause struct {
	Attribute string        `json:"attribute" bson:"attribute"`
	Op        operator      `json:"op" bson:"op"`
	Values    []interface{} `json:"values" bson:"values"` // An array, interpreted as an OR of values
	Negate    bool          `json:"negate" bson:"negate"`
}

// weightedVariation describes a fraction of users who will receive a specific variation.
type weightedVariation struct {
	Variation int `json:"variation" bson:"variation"`
	Weight    int `json:"weight" bson:"weight"` // Ranges from 0 to 100000
}

// target describes a set of users who will receive a specific variation.
type target struct {
	Values    []string `json:"values" bson:"values"`
	Variation int      `json:"variation" bson:"variation"`
}

// prerequisite describes a requirement that another feature flag return a specific variation.
type prerequisite struct {
	Key       string `json:"key"`
	Variation int    `json:"variation"`
}

func bucketUser(user User, key, attr, salt string) float32 {
	uValue, found := user.valueOf(attr)
	if !found {
		return 0
	}

	idHash, ok := bucketableStringValue(uValue)
	if !ok {
		return 0
	}

	if user.Secondary != nil {
		idHash = idHash + "." + *user.Secondary
	}

	h := sha1.New() // nolint:gas // just used for insecure hashing
	_, _ = io.WriteString(h, key+"."+salt+"."+idHash)
	hash := hex.EncodeToString(h.Sum(nil))[:15]

	intVal, _ := strconv.ParseInt(hash, 16, 64)

	bucket := float32(intVal) / longScale

	return bucket
}

func bucketableStringValue(uValue interface{}) (string, bool) {
	if s, ok := uValue.(string); ok {
		return s, true
	}
	// Can't only check for int type, because integer values in JSON may be decoded as float64
	if i, ok := uValue.(int); ok {
		return strconv.Itoa(i), true
	} else if i, ok := uValue.(float64); ok {
		if i == math.Trunc(i) {
			return strconv.Itoa(int(i)), true
		}
	}
	return "", false
}

// EvaluateDetail attempts to evaluate the feature flag for the given user and returns its
// value, the reason for the value, and any events generated by prerequisite flags.
func (f FeatureFlag) evaluateDetail(user User, store FeatureStore, sendReasonsInEvents bool) (EvaluationDetail, []FeatureRequestEvent) {
	if f.On {
		prereqErrorReason, prereqEvents := f.checkPrerequisites(user, store, sendReasonsInEvents)
		if prereqErrorReason != nil {
			return f.getOffValue(prereqErrorReason), prereqEvents
		}
		return f.evaluateInternal(user, store), prereqEvents
	}
	return f.getOffValue(evalReasonOffInstance), nil
}

// Returns nil if all prerequisites are OK, otherwise constructs an error reason that describes the failure
func (f FeatureFlag) checkPrerequisites(user User, store FeatureStore, sendReasonsInEvents bool) (EvaluationReason, []FeatureRequestEvent) {
	if len(f.Prerequisites) == 0 {
		return nil, nil
	}

	events := make([]FeatureRequestEvent, 0, len(f.Prerequisites))
	for _, prereq := range f.Prerequisites {
		data, err := store.Get(Features, prereq.Key)
		if err != nil || data == nil {
			return newEvalReasonPrerequisiteFailed(prereq.Key), events
		}
		prereqFeatureFlag, _ := data.(*FeatureFlag)
		prereqOK := true

		prereqResult, moreEvents := prereqFeatureFlag.evaluateDetail(user, store, sendReasonsInEvents)
		if !prereqFeatureFlag.On || prereqResult.VariationIndex == nil || *prereqResult.VariationIndex != prereq.Variation {
			// Note that if the prerequisite flag is off, we don't consider it a match no matter what its
			// off variation was. But we still need to evaluate it in order to generate an event.
			prereqOK = false
		}

		events = append(events, moreEvents...)
		prereqEvent := newSuccessfulEvalEvent(prereqFeatureFlag, user, prereqResult.VariationIndex,
			prereqResult.JSONValue, ldvalue.Null(), prereqResult.Reason, sendReasonsInEvents, &f.Key)
		if sendReasonsInEvents {
			prereqEvent.Reason.Reason = prereqResult.Reason
		}
		events = append(events, prereqEvent)

		if !prereqOK {
			return newEvalReasonPrerequisiteFailed(prereq.Key), events
		}
	}
	return nil, events
}

func (f FeatureFlag) evaluateInternal(user User, store FeatureStore) EvaluationDetail {
	// Check to see if targets match
	for _, target := range f.Targets {
		for _, value := range target.Values {
			if value == *user.Key {
				return f.getVariation(target.Variation, evalReasonTargetMatchInstance)
			}
		}
	}

	// Now walk through the rules and see if any match
	for ruleIndex, rule := range f.Rules {
		if rule.matchesUser(store, user) {
			reason := newEvalReasonRuleMatch(ruleIndex, rule.ID)
			return f.getValueForVariationOrRollout(rule.variationOrRollout, user, reason)
		}
	}

	return f.getValueForVariationOrRollout(f.Fallthrough, user, evalReasonFallthroughInstance)
}

func (f FeatureFlag) getVariation(index int, reason EvaluationReason) EvaluationDetail {
	if index < 0 || index >= len(f.Variations) {
		return EvaluationDetail{Reason: newEvalReasonError(EvalErrorMalformedFlag)}
	}
	value := f.Variations[index]
	return EvaluationDetail{
		Reason:         reason,
		Value:          value,
		JSONValue:      ldvalue.UnsafeUseArbitraryValue(value), //nolint:staticcheck // allow deprecated usage
		VariationIndex: &index,
	}
}

func (f FeatureFlag) getOffValue(reason EvaluationReason) EvaluationDetail {
	if f.OffVariation == nil {
		return EvaluationDetail{Reason: reason}
	}
	return f.getVariation(*f.OffVariation, reason)
}

func (f FeatureFlag) getValueForVariationOrRollout(vr variationOrRollout, user User, reason EvaluationReason) EvaluationDetail {
	index := vr.variationIndexForUser(user, f.Key, f.Salt)
	if index == nil {
		return EvaluationDetail{Reason: newEvalReasonError(EvalErrorMalformedFlag)}
	}
	return f.getVariation(*index, reason)
}

func (r flagRule) matchesUser(store FeatureStore, user User) bool {
	for _, clause := range r.Clauses {
		if !clause.matchesUser(store, user) {
			return false
		}
	}
	return true
}

func (c clause) matchesUserNoSegments(user User) bool {
	uValue, found := user.valueOf(c.Attribute)

	if !found {
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

func (c clause) matchesUser(store FeatureStore, user User) bool {
	// In the case of a segment match operator, we check if the user is in any of the segments,
	// and possibly negate
	if c.Op == operatorSegmentMatch {
		for _, value := range c.Values {
			if vStr, ok := value.(string); ok {
				data, _ := store.Get(Segments, vStr)
				// If segment is not found or the store got an error, data will be nil and we'll just fall through
				// the next block. Unfortunately we have no access to a logger here so this failure is silent.
				if segment, segmentOk := data.(*Segment); segmentOk {
					if segment.containsUser(user) {
						return c.maybeNegate(true)
					}
				}
			}
		}
		return c.maybeNegate(false)
	}

	return c.matchesUserNoSegments(user)
}

func (c clause) maybeNegate(b bool) bool {
	if c.Negate {
		return !b
	}
	return b
}

func matchAny(fn opFn, value interface{}, values []interface{}) bool {
	for _, v := range values {
		if fn(value, v) {
			return true
		}
	}
	return false
}

func (r variationOrRollout) variationIndexForUser(user User, key, salt string) *int {
	if r.Variation != nil {
		return r.Variation
	}
	if r.Rollout == nil {
		// This is an error (malformed flag); either Variation or rollout must be non-nil.
		return nil
	}

	bucketBy := userKey
	if r.Rollout.BucketBy != nil {
		bucketBy = *r.Rollout.BucketBy
	}

	var bucket = bucketUser(user, key, bucketBy, salt)
	var sum float32

	if len(r.Rollout.Variations) == 0 {
		// This is an error (malformed flag); there must be at least one weighted variation.
		return nil
	}
	for _, wv := range r.Rollout.Variations {
		sum += float32(wv.Weight) / 100000.0
		if bucket < sum {
			return &wv.Variation
		}
	}
	// The user's bucket value was greater than or equal to the end of the last bucket. This could happen due
	// to a rounding error, or due to the fact that we are scaling to 100000 rather than 99999, or the flag
	// data could contain buckets that don't actually add up to 100000. Rather than returning an error in
	// this case (or changing the scaling, which would potentially change the results for *all* users), we
	// will simply put the user in the last bucket.
	v := r.Rollout.Variations[len(r.Rollout.Variations)-1].Variation
	return &v
}
