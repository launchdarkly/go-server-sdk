package ldcomponents

import (
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/sharedtest"

	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
)

const testSdkKey = "test-sdk-key"

func basicClientContext() interfaces.ClientContext {
	return sharedtest.NewSimpleTestContext(testSdkKey)
}
