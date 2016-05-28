package ldtest

import (
	"github.com/launchdarkly/go-client"
)

type Scenario struct {
	Name         string                 `json:"name"`
	FeatureKey   string                 `json:"featureKey"`
	DefaultValue interface{}            `json:"defaultValue"`
	ValueType    string                 `json:"valueType"`
	TestCases    []TestCase             `json:"testCases"`
	FeatureFlags []ldclient.FeatureFlag `json:"featureFlags"`
}

type TestCase struct {
	ExpectedValue interface{}   `json:"expectedValue"`
	User          ldclient.User `json:"user"`
}

type EvalFeatureRequest struct {
	ValueType    string        `json:"valueType"`
	User         ldclient.User `json:"user"`
	DefaultValue interface{}   `json:"defaultValue"`
}

type EvalFeatureResponse struct {
	Key    string      `json:"key"`
	Result interface{} `json:"result"`
	Error  *string     `json:"error"`
}
