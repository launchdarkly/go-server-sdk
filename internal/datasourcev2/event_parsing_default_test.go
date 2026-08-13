//go:build !launchdarkly_easyjson
// +build !launchdarkly_easyjson

package datasourcev2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests document behavior specific to the default build's single-pass parser. The easyjson
// build's reflection-based decode parses the whole envelope up front, so it does not share them.

func TestParsePollingPayloadStopsReadingAfterTransfer(t *testing.T) {
	// Once payload-transferred completes the change set, the single-pass parser never reads the
	// rest of the body -- even content that is not valid JSON.
	body := []byte(`{"events":[` +
		`{"event":"server-intent","data":{"payloads":[{"id":"p1","target":1,"intentCode":"xfer-full","reason":"r"}]}},` +
		`{"event":"payload-transferred","data":{"state":"s","version":1}},` +
		`{"event":"put-object","data":{"this is": "not even valid put data`,
	)
	changeSet, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)
	assert.Empty(t, changeSet.Changes())
}
