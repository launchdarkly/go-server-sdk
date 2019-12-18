package ldclient

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

type diagnosticId struct {
	DiagnosticID string `json:"diagnosticId"`
	SDKKeySuffix string `json:"sdkKeySuffix,omitempty"`
}

type diagnosticSDKData struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	WrapperName    string `json:"wrapperName,omitempty"`
	WrapperVersion string `json:"wrapperVersion,omitempty"`
}

type diagnosticPlatformData struct {
	Name      string `json:"name"`
	GoVersion string `json:"goVersion"`
	OSArch    string `json:"osArch"`
	OSName    string `json:"osName"`
	OSVersion string `json:"osVersion"`
}

type milliseconds int

type diagnosticConfigData struct {
	CustomBaseURI               bool         `json:"customBaseURI"`
	CustomStreamURI             bool         `json:"customStreamURI"`
	CustomEventsURI             bool         `json:"customEventsURI"`
	EventsCapacity              int          `json:"eventsCapacity"`
	ConnectTimeoutMillis        milliseconds `json:"connectTimeoutMillis"`
	SocketTimeoutMillis         milliseconds `json:"socketTimeoutMillis"`
	EventsFlushIntervalMillis   milliseconds `json:"eventsFlushIntervalMillis"`
	PollingIntervalMillis       milliseconds `json:"pollingIntervalMillis"`
	StartWaitMillis             milliseconds `json:"startWaitMillis"`
	SamplingInterval            int32        `json:"samplingInterval"`
	ReconnectTimeMillis         milliseconds `json:"reconnectTimeMillis"`
	StreamingDisabled           bool         `json:"streamingDisabled"`
	UsingRelayDaemon            bool         `json:"usingRelayDaemon"`
	Offline                     bool         `json:"offline"`
	AllAttributesPrivate        bool         `json:"allAttributesPrivate"`
	InlineUsersInEvents         bool         `json:"inlineUsersInEvents"`
	UserKeysCapacity            int          `json:"userKeysCapacity"`
	UserKeysFlushIntervalMillis milliseconds `json:"userKeysFlushIntervalMillis"`
	UsingProxy                  bool         `json:"usingProxy"`
	// UsingProxyAuthenticator  bool         `json:"usingProxyAuthenticator"` // not knowable in Go SDK
	DiagnosticRecordingIntervalMillis milliseconds `json:"diagnosticRecordingIntervalMillis"`
}

type diagnosticBaseEvent struct {
	Kind         string       `json:"kind"`
	ID           diagnosticId `json:"id"`
	CreationDate uint64       `json:"creationDate"`
}

type diagnosticInitEvent struct {
	diagnosticBaseEvent
	SDK           diagnosticSDKData      `json:"sdk"`
	Configuration diagnosticConfigData   `json:"configuration"`
	Platform      diagnosticPlatformData `json:"platform"`
}

type diagnosticPeriodicEvent struct {
	diagnosticBaseEvent
	DataSinceDate     uint64                     `json:"dataSinceDate"`
	DroppedEvents     int                        `json:"droppedEvents"`
	DeduplicatedUsers int                        `json:"deduplicatedUsers"`
	EventsInLastBatch int                        `json:"eventsInLastBatch"`
	StreamInits       []diagnosticStreamInitInfo `json:"streamInits"`
}

type diagnosticStreamInitInfo struct {
	Timestamp      uint64       `json:"timestamp"`
	Failed         bool         `json:"failed"`
	DurationMillis milliseconds `json:"durationMillis"`
}

type diagnosticsManager struct {
	id            diagnosticId
	config        Config
	startWaitTime time.Duration // this is passed in separately because in Go, it's not part of the Config
	startTime     uint64
	dataSinceTime uint64
	streamInits   []diagnosticStreamInitInfo
	lock          sync.Mutex
}

func durationToMillis(d time.Duration) milliseconds {
	return milliseconds(d / time.Millisecond)
}

func newDiagnosticId(sdkKey string) diagnosticId {
	uuid, _ := uuid.NewRandom()
	id := diagnosticId{
		DiagnosticID: uuid.String(),
	}
	if len(sdkKey) > 6 {
		id.SDKKeySuffix = sdkKey[len(sdkKey)-6:]
	} else {
		id.SDKKeySuffix = sdkKey
	}
	return id
}

func newDiagnosticsManager(
	id diagnosticId,
	config Config,
	startWaitTime time.Duration,
	startTime time.Time,
) *diagnosticsManager {
	timestamp := toUnixMillis(startTime)
	m := &diagnosticsManager{
		id:            id,
		config:        config,
		startWaitTime: startWaitTime,
		startTime:     timestamp,
		dataSinceTime: timestamp,
	}
	return m
}

// Called by the stream processor when a stream connection has either succeeded or failed.
func (m *diagnosticsManager) RecordStreamInit(timestamp uint64, failed bool, durationMillis milliseconds) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.streamInits = append(m.streamInits, diagnosticStreamInitInfo{
		Timestamp:      timestamp,
		Failed:         failed,
		DurationMillis: durationMillis,
	})
}

// Called by DefaultEventProcessor to create the initial diagnostics event that includes the configuration.
func (m *diagnosticsManager) CreateInitEvent() diagnosticInitEvent {
	sdkData := diagnosticSDKData{
		Name:    "Go",
		Version: Version,
	}
	// Notes on configData
	// - reconnectTimeMillis: hard-coded in eventsource because we're not overriding StreamOptionInitialRetry.
	// - usingProxy: there are many ways to implement an HTTP proxy in Go, but the only one we're capable of
	//   detecting is the HTTP_PROXY environment variable; programmatic approaches involve using a custom
	//   transport, which we have no way of distinguishing from other kinds of custom transports (for the
	//   same reason, we cannot detect if proxy authentication is being used).
	configData := diagnosticConfigData{
		CustomBaseURI:                     m.config.BaseUri != DefaultConfig.BaseUri,
		CustomStreamURI:                   m.config.StreamUri != DefaultConfig.StreamUri,
		CustomEventsURI:                   m.config.EventsUri != DefaultConfig.EventsUri,
		EventsCapacity:                    m.config.Capacity,
		ConnectTimeoutMillis:              durationToMillis(m.config.Timeout),
		SocketTimeoutMillis:               durationToMillis(m.config.Timeout),
		EventsFlushIntervalMillis:         durationToMillis(m.config.FlushInterval),
		PollingIntervalMillis:             durationToMillis(m.config.PollInterval),
		StartWaitMillis:                   durationToMillis(m.startWaitTime),
		SamplingInterval:                  m.config.SamplingInterval,
		ReconnectTimeMillis:               3000,
		StreamingDisabled:                 !m.config.Stream,
		UsingRelayDaemon:                  m.config.UseLdd,
		Offline:                           m.config.Offline,
		AllAttributesPrivate:              m.config.AllAttributesPrivate,
		InlineUsersInEvents:               m.config.InlineUsersInEvents,
		UserKeysCapacity:                  m.config.UserKeysCapacity,
		UserKeysFlushIntervalMillis:       durationToMillis(m.config.UserKeysFlushInterval),
		UsingProxy:                        os.Getenv("HTTP_PROXY") != "",
		DiagnosticRecordingIntervalMillis: durationToMillis(m.config.DiagnosticRecordingInterval),
	}
	// Notes on platformData
	// - osArch: in Go, GOARCH is set at compile time, not at runtime (unlike GOOS, whiich is runtime).
	// - osVersion: Go provides no portable way to get this property.
	platformData := diagnosticPlatformData{
		Name:      "Go",
		GoVersion: runtime.Version(),
		OSName:    normalizeOSName(runtime.GOOS),
		OSArch:    runtime.GOARCH,
		//OSVersion: // not available, see above
	}
	return diagnosticInitEvent{
		diagnosticBaseEvent: diagnosticBaseEvent{
			Kind:         "diagnostic-init",
			ID:           m.id,
			CreationDate: m.startTime,
		},
		SDK:           sdkData,
		Configuration: configData,
		Platform:      platformData,
	}
}

// Called by DefaultEventProcessor to create the periodic event containing usage statistics. Some of the
// statistics are passed in as parameters because DefaultEventProcessor owns them and can more easily keep
// track of them internally - pushing them into diagnosticsManager would require frequent lock usage.
func (m *diagnosticsManager) CreateStatsEventAndReset(
	droppedEvents int,
	deduplicatedUsers int,
	eventsInLastBatch int,
) diagnosticPeriodicEvent {
	m.lock.Lock()
	defer m.lock.Unlock()
	timestamp := now()
	event := diagnosticPeriodicEvent{
		diagnosticBaseEvent: diagnosticBaseEvent{
			Kind:         "diagnostic",
			ID:           m.id,
			CreationDate: timestamp,
		},
		DataSinceDate:     m.dataSinceTime,
		EventsInLastBatch: eventsInLastBatch,
		DroppedEvents:     droppedEvents,
		DeduplicatedUsers: deduplicatedUsers,
		StreamInits:       m.streamInits,
	}
	m.streamInits = nil
	m.dataSinceTime = timestamp
	return event
}

func normalizeOSName(osName string) string {
	switch osName {
	case "darwin":
		return "MacOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return osName
}
