package ldcomponents

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-server-sdk/ldevents/v4"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v3/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note that we can't really test every event configuration option in these tests - they are tested in detail in
// the ldevents package, but we do want to verify that the basic options are being passed to ldevents correctly.

func TestEventProcessorBuilder(t *testing.T) {
	t.Run("AllAttributesPrivate", func(t *testing.T) {
		b := SendEvents()
		assert.False(t, b.allAttributesPrivate)

		b.AllAttributesPrivate(true)
		assert.True(t, b.allAttributesPrivate)

		b.AllAttributesPrivate(false)
		assert.False(t, b.allAttributesPrivate)
	})

	t.Run("Capacity", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultEventsCapacity, b.capacity)

		b.Capacity(333)
		assert.Equal(t, 333, b.capacity)
	})

	t.Run("DiagnosticRecordingInterval", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultDiagnosticRecordingInterval, b.diagnosticRecordingInterval)

		b.DiagnosticRecordingInterval(time.Hour)
		assert.Equal(t, time.Hour, b.diagnosticRecordingInterval)

		b.DiagnosticRecordingInterval(time.Second)
		assert.Equal(t, MinimumDiagnosticRecordingInterval, b.diagnosticRecordingInterval)
	})

	t.Run("FlushInterval", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultFlushInterval, b.flushInterval)

		b.FlushInterval(time.Hour)
		assert.Equal(t, time.Hour, b.flushInterval)
	})

	t.Run("PrivateAttributes", func(t *testing.T) {
		b := SendEvents()
		assert.Len(t, b.privateAttributes, 0)

		b.PrivateAttributes("name", "/address/street")
		assert.Equal(t, []ldattr.Ref{ldattr.NewRef("name"), ldattr.NewRef("/address/street")},
			b.privateAttributes)
	})

	t.Run("ContextKeysCapacity", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultContextKeysCapacity, b.contextKeysCapacity)

		b.ContextKeysCapacity(333)
		assert.Equal(t, 333, b.contextKeysCapacity)
	})

	t.Run("ContextKeysFlushInterval", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultContextKeysFlushInterval, b.contextKeysFlushInterval)

		b.ContextKeysFlushInterval(time.Hour)
		assert.Equal(t, time.Hour, b.contextKeysFlushInterval)
	})
}

func TestDefaultEventsConfigWithoutDiagnostics(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			Build(makeTestContextWithBaseURIs(server.URL))
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ce := ef.NewCustomEventData("event-key", ldevents.Context(lduser.NewUser("key")), ldvalue.Null(), false, 0, ldvalue.OptionalInt{})
		ep.RecordCustomEvent(ce)
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 2, jsonData.Count())
		assert.Equal(t, ldvalue.String("index"), jsonData.GetByIndex(0).GetByKey("kind"))
		assert.Equal(t, ldvalue.String("custom"), jsonData.GetByIndex(1).GetByKey("kind"))
	})
}

func TestDefaultEventsConfigWithDiagnostics(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	diagnosticsManager := ldevents.NewDiagnosticsManager(
		ldevents.NewDiagnosticID("sdk-key"),
		ldvalue.Null(),
		ldvalue.Null(),
		time.Now(),
		nil,
	)
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		context := makeTestContextWithBaseURIs(server.URL)
		context.DiagnosticsManager = diagnosticsManager
		_, err := SendEvents().
			Build(context)
		require.NoError(t, err)

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, ldvalue.String("diagnostic-init"), jsonData.GetByKey("kind"))
	})
}

func TestEventsAllAttributesPrivate(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			AllAttributesPrivate(true).
			Build(makeTestContextWithBaseURIs(server.URL))
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ie := ef.NewIdentifyEventData(ldevents.Context(lduser.NewUserBuilder("user-key").Name("user-name").Build()), ldvalue.OptionalInt{})
		ep.RecordIdentifyEvent(ie)
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count())
		event := jsonData.GetByIndex(0)
		m.In(t).Assert(event, m.AllOf(
			m.JSONProperty("kind").Should(m.Equal("identify")),
			m.JSONProperty("context").Should(m.AllOf(
				m.JSONProperty("key").Should(m.Equal("user-key")),
				m.JSONOptProperty("name").Should(m.BeNil()),
				m.JSONProperty("_meta").Should(m.JSONProperty("redactedAttributes").Should(m.JSONStrEqual(`["name"]`))),
			)),
		))
	})
}

func TestEventsCapacity(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			Capacity(1).
			Build(makeTestContextWithBaseURIs(server.URL))
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ie := ef.NewIdentifyEventData(ldevents.Context(lduser.NewUserBuilder("user-key").Name("user-name").Build()), ldvalue.OptionalInt{})
		ep.RecordIdentifyEvent(ie)
		ep.RecordIdentifyEvent(ie) // 2nd event will be dropped
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count())
	})
}

func TestEventsSomeAttributesPrivate(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			PrivateAttributes("name").
			Build(makeTestContextWithBaseURIs(server.URL))
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ie := ef.NewIdentifyEventData(ldevents.Context(lduser.NewUserBuilder("user-key").Email("user-email").Name("user-name").Build()), ldvalue.OptionalInt{})
		ep.RecordIdentifyEvent(ie)
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count())
		event := jsonData.GetByIndex(0)
		m.In(t).Assert(event, m.AllOf(
			m.JSONProperty("kind").Should(m.Equal("identify")),
			m.JSONProperty("context").Should(m.AllOf(
				m.JSONProperty("key").Should(m.Equal("user-key")),
				m.JSONProperty("email").Should(m.Equal("user-email")),
				m.JSONOptProperty("name").Should(m.BeNil()),
				m.JSONProperty("_meta").Should(m.JSONProperty("redactedAttributes").Should(m.JSONStrEqual(`["name"]`))),
			)),
		))
	})
}
