package ldstoreimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
)

func TestDataKinds(t *testing.T) {
	// Here we're just verifying that the public API returns the same instances that we're using internally.
	// The behavior of those instances is tested in internal/datakinds where they are implemented.

	assert.Equal(t, datakinds.Features, Features())
	assert.Equal(t, datakinds.Segments, Segments())
	assert.Equal(t, []ldstoretypes.DataKind{Features(), Segments()}, AllKinds())
}
