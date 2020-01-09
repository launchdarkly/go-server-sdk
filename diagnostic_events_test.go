package ldclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDiagnosticIDHasRandomID(t *testing.T) {
	id0 := newDiagnosticId("sdkkey")
	assert.NotEqual(t, "", id0.DiagnosticID)
	id1 := newDiagnosticId("sdkkey")
	assert.NotEqual(t, "", id1.DiagnosticID)
	assert.NotEqual(t, id0.DiagnosticID, id1.DiagnosticID)
}

func TestDiagnosticIDUsesLast6CharsOfSDKKey(t *testing.T) {
	id := newDiagnosticId("1234567890")
	assert.Equal(t, "567890", id.SDKKeySuffix)
}

func TestDiagnosticInitEventBaseProperties(t *testing.T) {
	id := newDiagnosticId("sdkkey")
	startTime := time.Now()
	m := newDiagnosticsManager(id, Config{}, time.Second, startTime, nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "diagnostic-init", event.Kind)
	assert.Equal(t, id, event.ID)
	assert.Equal(t, toUnixMillis(startTime), event.CreationDate)
}

func TestDiagnosticInitEventSDKData(t *testing.T) {
	id := newDiagnosticId("sdkkey")
	m := newDiagnosticsManager(id, Config{}, time.Second, time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "Go", event.SDK.Name)
	assert.Equal(t, Version, event.SDK.Version)
}

func TestDiagnosticInitEventPlatformData(t *testing.T) {
	id := newDiagnosticId("sdkkey")
	m := newDiagnosticsManager(id, Config{}, time.Second, time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, "Go", event.Platform.Name)
}

func TestDiagnosticInitEventDefaultConfig(t *testing.T) {
	id := newDiagnosticId("sdkkey")
	m := newDiagnosticsManager(id, DefaultConfig, 5*time.Second, time.Now(), nil)
	event := m.CreateInitEvent()
	assert.Equal(t, expectedDiagnosticConfigForDefaultConfig(), event.Configuration)
}

func expectedDiagnosticConfigForDefaultConfig() diagnosticConfigData {
	return diagnosticConfigData{
		CustomBaseURI:                     false,
		CustomStreamURI:                   false,
		CustomEventsURI:                   false,
		EventsCapacity:                    DefaultConfig.Capacity,
		ConnectTimeoutMillis:              durationToMillis(DefaultConfig.Timeout),
		SocketTimeoutMillis:               durationToMillis(DefaultConfig.Timeout),
		EventsFlushIntervalMillis:         durationToMillis(DefaultConfig.FlushInterval),
		PollingIntervalMillis:             durationToMillis(DefaultConfig.PollInterval),
		StartWaitMillis:                   milliseconds(5000),
		SamplingInterval:                  0,
		ReconnectTimeMillis:               3000,
		StreamingDisabled:                 false,
		UsingRelayDaemon:                  false,
		Offline:                           false,
		AllAttributesPrivate:              false,
		InlineUsersInEvents:               false,
		UserKeysCapacity:                  DefaultConfig.UserKeysCapacity,
		UserKeysFlushIntervalMillis:       durationToMillis(DefaultConfig.UserKeysFlushInterval),
		DiagnosticRecordingIntervalMillis: durationToMillis(DefaultConfig.DiagnosticRecordingInterval),
	}
}

func TestDiagnosticEventCustomConfig(t *testing.T) {
	id := newDiagnosticId("sdkkey")
	tests := []struct {
		setConfig   func(*Config)
		setExpected func(*diagnosticConfigData)
	}{
		{func(c *Config) { c.BaseUri = "custom" }, func(d *diagnosticConfigData) { d.CustomBaseURI = true }},
		{func(c *Config) { c.StreamUri = "custom" }, func(d *diagnosticConfigData) { d.CustomStreamURI = true }},
		{func(c *Config) { c.EventsUri = "custom" }, func(d *diagnosticConfigData) { d.CustomEventsURI = true }},
		{func(c *Config) { c.Capacity = 99 }, func(d *diagnosticConfigData) { d.EventsCapacity = 99 }},
		{func(c *Config) { c.Timeout = time.Second }, func(d *diagnosticConfigData) {
			d.ConnectTimeoutMillis = 1000
			d.SocketTimeoutMillis = 1000
		}},
		{func(c *Config) { c.FlushInterval = time.Second }, func(d *diagnosticConfigData) { d.EventsFlushIntervalMillis = 1000 }},
		{func(c *Config) { c.PollInterval = time.Second }, func(d *diagnosticConfigData) { d.PollingIntervalMillis = 1000 }},
		{func(c *Config) { c.SamplingInterval = 2 }, func(d *diagnosticConfigData) { d.SamplingInterval = 2 }},
		{func(c *Config) { c.Stream = false }, func(d *diagnosticConfigData) { d.StreamingDisabled = true }},
		{func(c *Config) { c.UseLdd = true }, func(d *diagnosticConfigData) { d.UsingRelayDaemon = true }},
		{func(c *Config) { c.AllAttributesPrivate = true }, func(d *diagnosticConfigData) { d.AllAttributesPrivate = true }},
		{func(c *Config) { c.InlineUsersInEvents = true }, func(d *diagnosticConfigData) { d.InlineUsersInEvents = true }},
		{func(c *Config) { c.UserKeysCapacity = 2 }, func(d *diagnosticConfigData) { d.UserKeysCapacity = 2 }},
		{func(c *Config) { c.UserKeysFlushInterval = time.Second }, func(d *diagnosticConfigData) { d.UserKeysFlushIntervalMillis = 1000 }},
		{func(c *Config) { c.DiagnosticRecordingInterval = time.Second }, func(d *diagnosticConfigData) { d.DiagnosticRecordingIntervalMillis = 1000 }},
	}
	for _, test := range tests {
		config := DefaultConfig
		test.setConfig(&config)
		expected := expectedDiagnosticConfigForDefaultConfig()
		test.setExpected(&expected)

		m := newDiagnosticsManager(id, config, 5*time.Second, time.Now(), nil)
		event := m.CreateInitEvent()
		assert.Equal(t, expected, event.Configuration)
	}
}
