package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalUpdatesOnly(t *testing.T) {
	ds, err := ExternalUpdatesOnly().CreateDataSource(basicClientContext(), nil, nil)
	require.NoError(t, err)
	defer ds.Close()
	assert.True(t, ds.Initialized())
	closeWhenReady := make(chan struct{})
	ds.Start(closeWhenReady)
	<-closeWhenReady
}
