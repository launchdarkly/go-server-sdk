package ldevents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func TestDiagnosticIDHasRandomID(t *testing.T) {
	id0 := NewDiagnosticID("sdkkey")
	key0 := id0.GetByKey("diagnosticId")
	assert.Equal(t, ldvalue.StringType, key0.Type())
	assert.NotEqual(t, "", key0.StringValue())
	id1 := NewDiagnosticID("sdkkey")
	key1 := id1.GetByKey("diagnosticId")
	assert.Equal(t, ldvalue.StringType, key1.Type())
	assert.NotEqual(t, "", key1.StringValue())
	assert.NotEqual(t, key0, key1)
}

func TestDiagnosticIDUsesLast6CharsOfSDKKey(t *testing.T) {
	id := NewDiagnosticID("1234567890")
	assert.Equal(t, "567890", id.GetByKey("sdkKeySuffix").StringValue())
}

func TestDiagnosticInitEventBaseProperties(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	startTime := time.Now()
	m := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), startTime, nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "diagnostic-init", event.GetByKey("kind").StringValue())
	assert.Equal(t, id, event.GetByKey("id"))
	assert.Equal(t, float64(toUnixMillis(startTime)), event.GetByKey("creationDate").Float64Value())
}

func TestDiagnosticInitEventConfigData(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	configData := ldvalue.ObjectBuild().Set("things", ldvalue.String("stuff")).Build()
	m := NewDiagnosticsManager(id, configData, ldvalue.Null(), time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, configData, event.GetByKey("configuration"))
}

func TestDiagnosticInitEventSDKData(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	sdkData := ldvalue.ObjectBuild().Set("name", ldvalue.String("my-sdk")).Build()
	m := NewDiagnosticsManager(id, ldvalue.Null(), sdkData, time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, sdkData, event.GetByKey("sdk"))
}

func TestDiagnosticInitEventPlatformData(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	m := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "Go", event.GetByKey("platform").GetByKey("name").StringValue())
}
