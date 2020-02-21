package ldclient

// func expectedDiagnosticConfigForDefaultConfig() diagnosticConfigData {
// 	return diagnosticConfigData{
// 		CustomBaseURI:                     false,
// 		CustomStreamURI:                   false,
// 		CustomEventsURI:                   false,
// 		EventsCapacity:                    DefaultConfig.Capacity,
// 		ConnectTimeoutMillis:              durationToMillis(DefaultConfig.Timeout),
// 		SocketTimeoutMillis:               durationToMillis(DefaultConfig.Timeout),
// 		EventsFlushIntervalMillis:         durationToMillis(DefaultConfig.FlushInterval),
// 		PollingIntervalMillis:             durationToMillis(DefaultConfig.PollInterval),
// 		StartWaitMillis:                   milliseconds(5000),
// 		ReconnectTimeMillis:               3000,
// 		StreamingDisabled:                 false,
// 		UsingRelayDaemon:                  false,
// 		Offline:                           false,
// 		AllAttributesPrivate:              false,
// 		InlineUsersInEvents:               false,
// 		UserKeysCapacity:                  DefaultConfig.UserKeysCapacity,
// 		UserKeysFlushIntervalMillis:       durationToMillis(DefaultConfig.UserKeysFlushInterval),
// 		DiagnosticRecordingIntervalMillis: durationToMillis(DefaultConfig.DiagnosticRecordingInterval),
// 	}
// }

// func TestDiagnosticEventCustomConfig(t *testing.T) {
// 	id := NewDiagnosticId("sdkkey")
// 	tests := []struct {
// 		setConfig   func(*Config)
// 		setExpected func(*diagnosticConfigData)
// 	}{
// 		{func(c *Config) { c.BaseUri = "custom" }, func(d *diagnosticConfigData) { d.CustomBaseURI = true }},
// 		{func(c *Config) { c.StreamUri = "custom" }, func(d *diagnosticConfigData) { d.CustomStreamURI = true }},
// 		{func(c *Config) { c.EventsUri = "custom" }, func(d *diagnosticConfigData) { d.CustomEventsURI = true }},
// 		{func(c *Config) {
// 			f := NewInMemoryDataStoreFactory()
// 			c.DataStore, _ = f(DefaultConfig)
// 		},
// 			func(d *diagnosticConfigData) {
// 				d.DataStoreType = ldvalue.NewOptionalString("memory")
// 			}},
// 		{func(c *Config) { c.DataStore = customStoreForDiagnostics{name: "Foo"} },
// 			func(d *diagnosticConfigData) {
// 				d.DataStoreType = ldvalue.NewOptionalString("Foo")
// 			}},
// 		// Can't use our actual persistent store implementations (Redis, etc.) in this test because it'd be
// 		// a circular package reference. There are tests in each of those packages to verify that they
// 		// return the expected component type names.
// 		{func(c *Config) { c.Capacity = 99 }, func(d *diagnosticConfigData) { d.EventsCapacity = 99 }},
// 		{func(c *Config) { c.Timeout = time.Second }, func(d *diagnosticConfigData) {
// 			d.ConnectTimeoutMillis = 1000
// 			d.SocketTimeoutMillis = 1000
// 		}},
// 		{func(c *Config) { c.FlushInterval = time.Second }, func(d *diagnosticConfigData) { d.EventsFlushIntervalMillis = 1000 }},
// 		{func(c *Config) { c.PollInterval = time.Second }, func(d *diagnosticConfigData) { d.PollingIntervalMillis = 1000 }},
// 		{func(c *Config) { c.Stream = false }, func(d *diagnosticConfigData) { d.StreamingDisabled = true }},
// 		{func(c *Config) { c.UseLdd = true }, func(d *diagnosticConfigData) { d.UsingRelayDaemon = true }},
// 		{func(c *Config) { c.AllAttributesPrivate = true }, func(d *diagnosticConfigData) { d.AllAttributesPrivate = true }},
// 		{func(c *Config) { c.InlineUsersInEvents = true }, func(d *diagnosticConfigData) { d.InlineUsersInEvents = true }},
// 		{func(c *Config) { c.UserKeysCapacity = 2 }, func(d *diagnosticConfigData) { d.UserKeysCapacity = 2 }},
// 		{func(c *Config) { c.UserKeysFlushInterval = time.Second }, func(d *diagnosticConfigData) { d.UserKeysFlushIntervalMillis = 1000 }},
// 		{func(c *Config) { c.DiagnosticRecordingInterval = time.Second }, func(d *diagnosticConfigData) { d.DiagnosticRecordingIntervalMillis = 1000 }},
// 	}
// 	for _, test := range tests {
// 		config := DefaultConfig
// 		test.setConfig(&config)
// 		expected := expectedDiagnosticConfigForDefaultConfig()
// 		test.setExpected(&expected)

// 		m := newDiagnosticsManager(id, config, 5*time.Second, time.Now(), nil)
// 		event := m.CreateInitEvent()
// 		assert.Equal(t, expected, event.Configuration)
// 	}
// }

// type customStoreForDiagnostics struct {
// 	name string
// }

// func (c customStoreForDiagnostics) GetDiagnosticsComponentTypeName() string {
// 	return c.name
// }

// func (c customStoreForDiagnostics) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
// 	return nil, nil
// }

// func (c customStoreForDiagnostics) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
// 	return nil, nil
// }

// func (c customStoreForDiagnostics) Init(data map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
// 	return nil
// }

// func (c customStoreForDiagnostics) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
// 	return nil
// }

// func (c customStoreForDiagnostics) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
// 	return nil
// }

// func (c customStoreForDiagnostics) Initialized() bool {
// 	return false
// }
