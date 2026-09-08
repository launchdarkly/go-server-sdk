package diagnostics

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
)

type componentWithoutDescription struct{}

type componentWithContext struct{}

func (c componentWithContext) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.String("with context: " + context.GetSDKKey())
}

type componentWithoutContext struct{}

func (c componentWithoutContext) DescribeConfiguration() ldvalue.Value {
	return ldvalue.String("without context")
}

func TestDescribeConfiguration(t *testing.T) {
	context := subsystems.BasicClientContext{SDKKey: "my-key"}

	t.Run("component does not describe itself", func(t *testing.T) {
		assert.Equal(t, ldvalue.Null(), DescribeConfiguration(componentWithoutDescription{}, context))
	})

	t.Run("component takes the context", func(t *testing.T) {
		assert.Equal(t, ldvalue.String("with context: my-key"),
			DescribeConfiguration(componentWithContext{}, context))
	})

	t.Run("component omits the context", func(t *testing.T) {
		assert.Equal(t, ldvalue.String("without context"),
			DescribeConfiguration(componentWithoutContext{}, context))
	})

	t.Run("nil component", func(t *testing.T) {
		assert.Equal(t, ldvalue.Null(), DescribeConfiguration(nil, context))
	})
}
