package internal

import (
	"net/http"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/stretchr/testify/assert"
)

func TestInitMetadata(t *testing.T) {
	t.Run("handles nil headers", func(t *testing.T) {
		initMetadata := NewInitMetadataFromHeaders(nil)
		assert.Equal(t, ldvalue.OptionalString{}, initMetadata.GetEnvironmentID())
	})

	t.Run("handles missing X-Ld-Envid header", func(t *testing.T) {
		initMetadata := NewInitMetadataFromHeaders(http.Header{})
		assert.Equal(t, ldvalue.OptionalString{}, initMetadata.GetEnvironmentID())
	})

	t.Run("finds environment id from X-Ld-Envid header", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("X-Ld-Envid", "test-env-id")
		initMetadata := NewInitMetadataFromHeaders(headers)
		assert.Equal(t, ldvalue.NewOptionalString("test-env-id"), initMetadata.GetEnvironmentID())
	})
}
