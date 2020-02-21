package ldevents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func TestDiagnosticIDHasRandomID(t *testing.T) {
	id0 := NewDiagnosticId("sdkkey")
	assert.NotEqual(t, "", id0.DiagnosticID)
	id1 := NewDiagnosticId("sdkkey")
	assert.NotEqual(t, "", id1.DiagnosticID)
	assert.NotEqual(t, id0.DiagnosticID, id1.DiagnosticID)
}

func TestDiagnosticIDUsesLast6CharsOfSDKKey(t *testing.T) {
	id := NewDiagnosticId("1234567890")
	assert.Equal(t, "567890", id.SDKKeySuffix)
}

func TestDiagnosticInitEventBaseProperties(t *testing.T) {
	id := NewDiagnosticId("sdkkey")
	startTime := time.Now()
	m := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), startTime, nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "diagnostic-init", event.Kind)
	assert.Equal(t, id, event.ID)
	assert.Equal(t, toUnixMillis(startTime), event.CreationDate)
}

func TestDiagnosticInitEventConfigData(t *testing.T) {
	id := NewDiagnosticId("sdkkey")
	configData := ldvalue.ObjectBuild().Set("things", ldvalue.String("stuff")).Build()
	m := NewDiagnosticsManager(id, configData, ldvalue.Null(), time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, configData, event.Configuration)
}

func TestDiagnosticInitEventSDKData(t *testing.T) {
	id := NewDiagnosticId("sdkkey")
	sdkData := ldvalue.ObjectBuild().Set("name", ldvalue.String("my-sdk")).Build()
	m := NewDiagnosticsManager(id, ldvalue.Null(), sdkData, time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, sdkData, event.SDK)
}

func TestDiagnosticInitEventPlatformData(t *testing.T) {
	id := NewDiagnosticId("sdkkey")
	m := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "Go", event.Platform.Name)
}
