package redis

import (
	"encoding/json"
	"testing"
	"time"

	r "github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/assert"
	ld "gopkg.in/launchdarkly/go-client.v3"
	ldtest "gopkg.in/launchdarkly/go-client.v3/shared_test"
)

func makeRedisStore() ld.FeatureStore {
	return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
}

func TestRedisFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeRedisStore)
}

func TestUpsertRaceConditionAgainstExternalClient(t *testing.T) {
	store := NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	otherClient, err := r.DialURL("redis://localhost:6379")
	assert.NoError(t, err)
	defer otherClient.Close()

	feature1 := ld.FeatureFlag{
		Key:     "foo",
		Version: 1,
	}
	intermediateVer := feature1
	finalVer := ld.FeatureFlag{
		Key:     "foo",
		Version: 10,
	}
	allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
		ld.Features: map[string]ld.VersionedData{feature1.Key: &feature1},
	}
	store.Init(allData)

	store.testTxHook = func() {
		intermediateVer.Version++
		if intermediateVer.Version < 5 {
			data, jsonErr := json.Marshal(intermediateVer)
			assert.NoError(t, jsonErr)
			otherClient.Do("HSET", "launchdarkly:features", feature1.Key, data)
		}
	}

	store.Upsert(ld.Features, &finalVer)
	var result ld.VersionedData
	result, err = store.Get(ld.Features, feature1.Key)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, finalVer.Version, result.(*ld.FeatureFlag).Version)
}
