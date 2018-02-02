package ldclient

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVariationIndexForUser(t *testing.T) {
	wv1 := WeightedVariation{Variation: 0, Weight: 60000.0}
	wv2 := WeightedVariation{Variation: 1, Weight: 40000.0}
	rollout := Rollout{Variations: []WeightedVariation{wv1, wv2}}
	rule := Rule{VariationOrRollout: VariationOrRollout{Rollout: &rollout}}

	userKey := "userKeyA"
	variationIndex := rule.variationIndexForUser(User{Key: &userKey}, "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)

	userKey = "userKeyB"
	variationIndex = rule.variationIndexForUser(User{Key: &userKey}, "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 1, *variationIndex)

	userKey = "userKeyC"
	variationIndex = rule.variationIndexForUser(User{Key: &userKey}, "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)
}

func TestBucketUser(t *testing.T) {
	userKey := "userKeyA"
	user := User{Key: &userKey}
	bucket := bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.42157587, bucket, 0.0000001)

	userKey = "userKeyB"
	user = User{Key: &userKey}
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.6708485, bucket, 0.0000001)

	userKey = "userKeyC"
	user = User{Key: &userKey}
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.10343106, bucket, 0.0000001)

	userKey = "userKeyD"
	custom := map[string]interface{}{
		"intAttr": 3,
	}
	user = User{Key: &userKey, Custom: &custom}
	bucket = bucketUser(user, "hashKey", "intAttr", "saltyA")
	assert.InEpsilon(t, 0.0073090503, bucket, 0.0000001)

	userKey = "userKeyE"
	custom = map[string]interface{}{
		"floatAttr": 999.999,
	}
	user = User{Key: &userKey, Custom: &custom}
	bucket = bucketUser(user, "hashKey", "floatAttr", "saltyA")
	assert.InDelta(t, 0.0, bucket, 0.0000001)
}
