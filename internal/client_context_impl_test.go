package internal

import (
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
)

func TestDataKindDeserializeOptions(t *testing.T) {
	release := datakinds.NewDeserializeOptions(true)
	assert.Equal(t, release, DataKindDeserializeOptions(&ClientContextImpl{ReleaseClauseValues: true}))
	assert.Equal(t, release, DataKindDeserializeOptions(ClientContextImpl{ReleaseClauseValues: true}))
	assert.Equal(t, datakinds.DeserializeOptions{}, DataKindDeserializeOptions(&ClientContextImpl{}))
	assert.Equal(t, datakinds.DeserializeOptions{}, DataKindDeserializeOptions(ClientContextImpl{}))
	assert.Equal(t, datakinds.DeserializeOptions{}, DataKindDeserializeOptions(subsystems.BasicClientContext{}))
}
