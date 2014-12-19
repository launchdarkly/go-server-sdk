package ldclient

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gregjones/httpcache"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	long_scale = float32(0xFFFFFFFFFFFFFFF)
)

var Version string = "0.0.3"

type User struct {
	Key       *string                 `json:"key,omitempty" bson:"key,omitempty"`
	Secondary *string                 `json:"secondary,omitempty" bson:"secondary,omitempty"`
	Ip        *string                 `json:"ip,omitempty" bson:"ip,omitempty"`
	Country   *string                 `json:"country,omitempty" bson:"country,omitempty"`
	Custom    *map[string]interface{} `json:"custom,omitempty" bson:"custom,omitempty"`
}

type Operator string

type Feature struct {
	Name         *string      `json:"name"`
	Key          *string      `json:"key"`
	Kind         *string      `json:"kind"`
	Salt         *string      `json:"salt"`
	On           *bool        `json:"on"`
	Variations   *[]Variation `json:"variations"`
	Ttl          *int         `json:"ttl"`
	CommitDate   *time.Time   `json:"commitDate"`
	CreationDate *time.Time   `json:"creationDate"`
}

type TargetRule struct {
	Attribute string        `json:"attribute"`
	Op        Operator      `json:"op"`
	Values    []interface{} `json:"values"`
}

type Variation struct {
	Value   interface{}  `json:"value"`
	Weight  int          `json:"weight"`
	Targets []TargetRule `json:"targets"`
}

type LDClient struct {
	apiKey     string
	config     Config
	httpClient *http.Client
	processor  *eventProcessor
}

type Config struct {
	BaseUri       string
	Capacity      int
	FlushInterval time.Duration
}

var DefaultConfig = Config{
	BaseUri:       "https://app.launchdarkly.com",
	Capacity:      1000,
	FlushInterval: 5 * time.Second,
}

func MakeCustomClient(apiKey string, config Config) LDClient {
	config.BaseUri = strings.TrimRight(config.BaseUri, "/")
	return LDClient{
		apiKey:     apiKey,
		config:     config,
		httpClient: httpcache.NewMemoryCacheTransport().Client(),
		processor:  newEventProcessor(apiKey, config),
	}
}

func MakeClient(apiKey string) *LDClient {
	res := MakeCustomClient(apiKey, DefaultConfig)
	return &res
}

func (b Feature) paramForId(user User) (float32, bool) {
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
	io.WriteString(h, *b.Key+"."+*b.Salt+"."+idHash)
	hash := hex.EncodeToString(h.Sum(nil))[:15]

	intVal, _ := strconv.ParseInt(hash, 16, 64)

	param := float32(intVal) / long_scale

	return param, false
}

func matchCustom(target TargetRule, user User) bool {
	if user.Custom == nil {
		return false
	}
	var v interface{} = (*user.Custom)[target.Attribute]

	if v == nil {
		return false
	}

	val := reflect.ValueOf(v)

	if val.Kind() == reflect.Array || val.Kind() == reflect.Slice {
		for i := 0; i < val.Len(); i++ {
			if compareValues(val.Index(i).Interface(), target.Values) {
				return true
			}
		}
		return false
	} else {
		return compareValues(v, target.Values)
	}
}

func compareValues(value interface{}, values []interface{}) bool {
	if value == "" {
		return false
	} else {
		for _, v := range values {
			if value == v {
				return true
			}
		}
	}
	return false
}

func matchTarget(targets []TargetRule, user User) bool {
	for _, target := range targets {
		var uValue string
		if target.Attribute == "key" {
			if user.Key != nil {
				uValue = *user.Key
			}
		} else if target.Attribute == "ip" {
			if user.Ip != nil {
				uValue = *user.Ip
			}
		} else if target.Attribute == "country" {
			if user.Country != nil {
				uValue = *user.Country
			}
		} else {
			if matchCustom(target, user) {
				return true
			} else {
				continue
			}
		}

		if compareValues(uValue, target.Values) {
			return true
		} else {
			continue
		}
	}
	return false
}

func (f Feature) Evaluate(user User) (interface{}, bool) {

	if !*f.On {
		return nil, true
	}

	param, passErr := f.paramForId(user)

	if passErr {
		return nil, true
	}

	for _, variation := range *f.Variations {
		if matchTarget(variation.Targets, user) {
			return variation.Value, false
		}
	}

	var sum float32 = 0.0

	for _, variation := range *f.Variations {
		sum += float32(variation.Weight) / 100.0
		if param < sum {
			return variation.Value, false
		}
	}

	return nil, true
}

func (client *LDClient) Track(key string, user User, data interface{}) error {
	evt := newCustomEvent(key, user, data)
	return client.processor.sendEvent(evt)
}

func (client *LDClient) Close() {
	client.processor.close()
}

func (client *LDClient) GetFlag(key string, user User, defaultVal bool) (bool, error) {

	req, reqErr := http.NewRequest("GET", client.config.BaseUri+"/api/eval/features/"+key, nil)

	if reqErr != nil {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, reqErr
	}

	req.Header.Add("Authorization", "api_key "+client.apiKey)
	req.Header.Add("User-Agent", "GoClient/"+Version)

	res, resErr := client.httpClient.Do(req)
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, errors.New("Invalid API key. Verify that your API key is correct. Returning default value.")
	}

	if res.StatusCode == http.StatusNotFound {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, errors.New("Invalid feature key. Verify that this feature key exists. Returning default value.")
	}

	if res.StatusCode != http.StatusOK {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, errors.New("Unexpected response code: " + strconv.Itoa(res.StatusCode))
	}

	if resErr != nil {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, resErr
	}

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, err
	}

	var feature Feature
	jsonErr := json.Unmarshal(body, &feature)

	if jsonErr != nil {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, jsonErr
	}

	value, pass := feature.Evaluate(user)

	if pass {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, nil
	}

	result, ok := value.(bool)

	if !ok {
		client.processor.sendEvent(newFeatureRequestEvent(key, user, defaultVal))
		return defaultVal, errors.New("Feature flag returned non-bool value")
	}

	client.processor.sendEvent(newFeatureRequestEvent(key, user, result))
	return result, nil
}
