package ldredis

import (
	"fmt"
	"strings"
	"testing"

	r "github.com/garyburd/redigo/redis"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

const redisURL = "redis://localhost:6379"

func TestRedisDataStore(t *testing.T) {
	sharedtest.NewPersistentDataStoreTestSuite(makeTestStore, clearTestData).
		ConcurrentModificationHook(setConcurrentModificationHook).
		Run(t)
}

func makeTestStore(prefix string) interfaces.PersistentDataStoreFactory {
	return DataStore().Prefix(prefix)
}

func clearTestData(prefix string) error {
	if prefix == "" {
		prefix = DefaultPrefix
	}

	client, err := r.DialURL(redisURL)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.Do("SCAN", "0", "MATCH", prefix+":*")
	if err != nil {
		return err
	}
	respValue, err := parseRedisResponseAsValue(resp)
	if err != nil {
		return err
	}
	if respValue.Count() == 2 {
		respLines := respValue.GetByIndex(1)
		if respLines.Type() == ldvalue.ArrayType {
			var failure error
			respLines.Enumerate(func(i int, key string, value ldvalue.Value) bool {
				redisKey := strings.TrimPrefix(strings.TrimSuffix(value.String(), `"`), `"`)
				failure = client.Send("DEL", redisKey)
				return failure == nil
			})
			if failure != nil {
				return failure
			}
			return client.Flush()
		}
	}
	return fmt.Errorf("unexpected format of Redis response: %s", respValue)
}

func setConcurrentModificationHook(store interfaces.PersistentDataStore, hook func()) {
	store.(*redisDataStoreImpl).testTxHook = hook
}

func parseRedisResponseAsValue(resp interface{}) (ldvalue.Value, error) {
	switch t := resp.(type) {
	case []interface{}:
		a := ldvalue.ArrayBuild()
		for _, item := range t {
			v, err := parseRedisResponseAsValue(item)
			if err != nil {
				return ldvalue.Null(), err
			}
			a.Add(v)
		}
		return a.Build(), nil
	case []byte:
		return ldvalue.String(string(t)), nil
	default:
		return ldvalue.Null(), fmt.Errorf("unexpected data type in response: %T", resp)
	}
}
