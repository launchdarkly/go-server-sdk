package ldclient

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"github.com/gregjones/httpcache"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"time"
)

const (
	long_scale = float32(0xFFFFFFFFFFFFFFF)
)

var Version string

type User struct {
	Key       *string                 `json:"key,omitempty"`
	Secondary *string                 `json:"secondary,omitempty"`
	Ip        *string                 `json:"ip,omitempty"`
	Country   *string                 `json:"country,omitempty"`
	Custom    *map[string]interface{} `json"custom,omitempty"`
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
	Attribute string   `json:"attribute"`
	Op        Operator `json:"op"`
	Values    []string `json:"values"`
}

type Variation struct {
	Value   interface{}  `json:"value"`
	Weight  int          `json:"weight"`
	Targets []TargetRule `json:"targets"`
}

type LDClient struct {
	ApiKey     string
	config     Config
	httpClient *http.Client
}

type Config struct {
	BaseUri string
}

func MakeCustomClient(apiKey string, config Config) LDClient {
	return LDClient{
		ApiKey:     apiKey,
		config:     config,
		httpClient: httpcache.NewMemoryCacheTransport().Client(),
	}
}

func MakeClient(apiKey string) LDClient {
	return MakeCustomClient(apiKey, Config{
		BaseUri: "https://app.launchdarkly.com",
	})
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

	switch v.(type) {
	case string:
		return compareValues(v.(string), target.Values)
	case []interface{}:
		for _, cVal := range v.([]interface{}) {
			if reflect.TypeOf(cVal).Kind() == reflect.String && compareValues(cVal.(string), target.Values) {
				return true
			}
		}
	default:
		return false
	}
	return false
}

func compareValues(value string, values []string) bool {
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

func (client LDClient) GetFlag(key string, user User, defaultVal bool) bool {

	req, reqErr := http.NewRequest("GET", client.config.BaseUri+"/api/eval/features/"+key, nil)

	if reqErr != nil {
		// TODO log error here
		return defaultVal
	}

	req.Header.Add("Authorization", "api_key "+client.ApiKey)
	req.Header.Add("User-Agent", "GoClient/"+Version)

	res, _ := client.httpClient.Do(req)
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		// TODO log error here
		return defaultVal
	}

	var feature Feature
	jsonErr := json.Unmarshal(body, &feature)

	if jsonErr != nil {
		// TODO log error here
		return defaultVal
	}

	value, pass := feature.Evaluate(user)

	if pass {
		return defaultVal
	}

	result, ok := value.(bool)

	if !ok {
		// TODO log error here
		return defaultVal
	}
	return result
}
