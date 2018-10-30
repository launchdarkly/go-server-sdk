package redis

import (
	"encoding/json"
	"testing"
	"time"

	r "github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/assert"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func makeRedisStore() ld.FeatureStore {
	return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
}

func TestRedisFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeRedisStore)
}

func concurrentModificationFunction(t *testing.T, otherClient r.Conn, flag ld.FeatureFlag,
	startVersion int, endVersion int) func() {
	versionCounter := startVersion
	return func() {
		if versionCounter <= endVersion {
			flag.Version = versionCounter
			data, jsonErr := json.Marshal(flag)
			assert.NoError(t, jsonErr)
			_, err := otherClient.Do("HSET", "launchdarkly:features", flag.Key, data)
			assert.NoError(t, err)
			versionCounter++
		}
	}
}

func TestUpsertRaceConditionAgainstExternalClientWithLowerVersion(t *testing.T) {
	store := NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	otherClient, err := r.DialURL("redis://localhost:6379")
	assert.NoError(t, err)
	defer otherClient.Close()

	flag := ld.FeatureFlag{
		Key:     "foo",
		Version: 1,
	}
	allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
		ld.Features: {flag.Key: &flag},
	}
	assert.NoError(t, store.Init(allData))

	store.testTxHook = concurrentModificationFunction(t, otherClient, flag, 2, 4)

	flag.Version = 10
	assert.NoError(t, store.Upsert(ld.Features, &flag))

	var result ld.VersionedData
	result, err = store.Get(ld.Features, flag.Key)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 10, result.(*ld.FeatureFlag).Version)
}

func TestUpsertRaceConditionAgainstExternalClientWithHigherVersion(t *testing.T) {
	store := NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	otherClient, err := r.DialURL("redis://localhost:6379")
	assert.NoError(t, err)
	defer otherClient.Close()

	flag := ld.FeatureFlag{
		Key:     "foo",
		Version: 1,
	}
	allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
		ld.Features: {flag.Key: &flag},
	}
	assert.NoError(t, store.Init(allData))

	store.testTxHook = concurrentModificationFunction(t, otherClient, flag, 3, 3)

	flag.Version = 2
	assert.NoError(t, store.Upsert(ld.Features, &flag))

	var result ld.VersionedData
	result, err = store.Get(ld.Features, flag.Key)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.(*ld.FeatureFlag).Version)
}
