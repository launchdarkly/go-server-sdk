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
	ExpectError   bool          `json:"expectError"`
	User          ldclient.User `json:"user"`
}

type ToggleFeatureRequest struct {
	User         ldclient.User `json:"user"`
	DefaultValue bool          `json:"defaultValue"`
}

type ToggleFeatureResponse struct {
	Key    string `json:"key"`
	Result bool   `json:"result"`
	Error  *string `json:"error"`
}
