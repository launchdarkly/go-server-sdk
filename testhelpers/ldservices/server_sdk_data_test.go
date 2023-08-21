package ldservices

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyAndVersionItem(t *testing.T) {
	f := KeyAndVersionItem("my-key", 2)
	bytes, err := json.Marshal(f)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"key":"my-key","version":2}`, string(bytes))
}

func TestEmptyServerSDKData(t *testing.T) {
	expectedJSON := `{"flags":{},"segments":{},"configurationOverrides":{},"metrics":{}}`
	data := NewServerSDKData()
	bytes, err := json.Marshal(data)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedJSON, string(bytes))
}

func TestSDKDataWithAllDataKinds(t *testing.T) {
	flag1 := KeyAndVersionItem("flagkey1", 1)
	flag2 := KeyAndVersionItem("flagkey2", 2)
	segment1 := KeyAndVersionItem("segkey1", 3)
	segment2 := KeyAndVersionItem("segkey2", 4)
	override1 := KeyAndVersionItem("override1", 5)
	override2 := KeyAndVersionItem("override2", 6)
	metric1 := KeyAndVersionItem("metric1", 7)
	metric2 := KeyAndVersionItem("metric2", 8)
	data := NewServerSDKData().Flags(flag1, flag2).Segments(segment1, segment2).ConfigOverrides(override1, override2).Metrics(metric1, metric2)

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
		},
		"configurationOverrides": {
			"override1": {
				"key": "override1",
				"version": 5
			},
			"override2": {
				"key": "override2",
				"version": 6
			}
		},
		"metrics": {
			"metric1": {
				"key": "metric1",
				"version": 7
			},
			"metric2": {
				"key": "metric2",
				"version": 8
			}
		}
	}`
	bytes, err := json.Marshal(data)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedJSON, string(bytes))
}
