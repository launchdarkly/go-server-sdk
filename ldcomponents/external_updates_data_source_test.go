package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalUpdatesOnly(t *testing.T) {
	ds, err := ExternalUpdatesOnly().CreateDataSource(basicClientContext(), nil)
	require.NoError(t, err)
	defer ds.Close()
	assert.True(t, ds.IsInitialized())
	closeWhenReady := make(chan struct{})
	ds.Start(closeWhenReady)
	waitForReadyWithTimeout(t, closeWhenReady, time.Second)
}
