package ldservices

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlagOrSegment(t *testing.T) {
	f := FlagOrSegment("my-key", 2)
	bytes, err := json.Marshal(f)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"key":"my-key","version":2}`, string(bytes))
}

func TestEmptyServerSDKData(t *testing.T) {
	expectedJSON := `{"flags":{},"segments":{}}`
	data := NewServerSDKData()
	bytes, err := json.Marshal(data)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedJSON, string(bytes))
}

func TestSDKDataWithFlagsAndSegments(t *testing.T) {
	flag1 := FlagOrSegment("flagkey1", 1)
	flag2 := FlagOrSegment("flagkey2", 2)
	segment1 := FlagOrSegment("segkey1", 3)
	segment2 := FlagOrSegment("segkey2", 4)
	data := NewServerSDKData().Flags(flag1, flag2).Segments(segment1, segment2)

	expectedJSON := `{
		"flags": {
			"flagkey1": {
				"key": "flagkey1",
				"version": 1
			},
			"flagkey2": {
				"key": "flagkey2",
				"version": 2
			}
		},
		"segments": {
			"segkey1": {
				"key": "segkey1",
				"version": 3
			},
			"segkey2": {
				"key": "segkey2",
				"version": 4
			}
		}
	}`
	bytes, err := json.Marshal(data)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedJSON, string(bytes))
}
