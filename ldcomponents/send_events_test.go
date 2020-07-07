package ldcomponents

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/ldservices"
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

	t.Run("BaseURI", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultEventsBaseURI, b.baseURI)

		b.BaseURI("x")
		assert.Equal(t, "x", b.baseURI)

		b.BaseURI("")
		assert.Equal(t, DefaultEventsBaseURI, b.baseURI)
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

	t.Run("InlineUsersInEvents", func(t *testing.T) {
		b := SendEvents()
		assert.False(t, b.inlineUsersInEvents)

		b.InlineUsersInEvents(true)
		assert.True(t, b.inlineUsersInEvents)

		b.InlineUsersInEvents(false)
		assert.False(t, b.inlineUsersInEvents)
	})

	t.Run("PrivateAttributeNames", func(t *testing.T) {
		b := SendEvents()
		assert.Len(t, b.privateAttributeNames, 0)

		b.PrivateAttributeNames(lduser.NameAttribute, lduser.UserAttribute("other"))
		assert.Equal(t, []lduser.UserAttribute{lduser.NameAttribute, lduser.UserAttribute("other")},
			b.privateAttributeNames)
	})

	t.Run("UserKeysCapacity", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultUserKeysCapacity, b.userKeysCapacity)

		b.UserKeysCapacity(333)
		assert.Equal(t, 333, b.userKeysCapacity)
	})

	t.Run("UserKeysFlushInterval", func(t *testing.T) {
		b := SendEvents()
		assert.Equal(t, DefaultUserKeysFlushInterval, b.userKeysFlushInterval)

		b.UserKeysFlushInterval(time.Hour)
		assert.Equal(t, time.Hour, b.userKeysFlushInterval)
	})
}

func TestDefaultEventsConfigWithoutDiagnostics(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			BaseURI(server.URL).
			CreateEventProcessor(basicClientContext())
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ce := ef.NewCustomEvent("event-key", ldevents.User(lduser.NewUser("key")), ldvalue.Null(), false, 0)
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
	context := sharedtest.NewClientContextWithDiagnostics("sdk-key", nil, nil, diagnosticsManager)
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		_, err := SendEvents().
			BaseURI(server.URL).
			CreateEventProcessor(context)
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
			BaseURI(server.URL).
			CreateEventProcessor(basicClientContext())
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ie := ef.NewIdentifyEvent(ldevents.User(lduser.NewUserBuilder("user-key").Name("user-name").Build()))
		ep.RecordIdentifyEvent(ie)
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count())
		event := jsonData.GetByIndex(0)
		assert.Equal(t, ldvalue.String("identify"), event.GetByKey("kind"))
		assert.Equal(t, ldvalue.String("user-key"), event.GetByKey("user").GetByKey("key"))
		assert.Equal(t, ldvalue.Null(), event.GetByKey("user").GetByKey("name"))
		assert.Equal(t, ldvalue.ArrayOf(ldvalue.String("name")), event.GetByKey("user").GetByKey("privateAttrs"))
	})
}

func TestEventsCapacity(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			BaseURI(server.URL).
			Capacity(1).
			CreateEventProcessor(basicClientContext())
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ie := ef.NewIdentifyEvent(ldevents.User(lduser.NewUserBuilder("user-key").Name("user-name").Build()))
		ep.RecordIdentifyEvent(ie)
		ep.RecordIdentifyEvent(ie) // 2nd event will be dropped
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count())
	})
}

func TestEventsInlineUsers(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			BaseURI(server.URL).
			InlineUsersInEvents(true).
			CreateEventProcessor(basicClientContext())
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ce := ef.NewCustomEvent("event-key", ldevents.User(lduser.NewUser("key")), ldvalue.Null(), false, 0)
		ep.RecordCustomEvent(ce)
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count()) // no index event
		assert.Equal(t, ldvalue.String("custom"), jsonData.GetByIndex(0).GetByKey("kind"))
	})
}

func TestEventsSomeAttributesPrivate(t *testing.T) {
	eventsHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(server *httptest.Server) {
		ep, err := SendEvents().
			BaseURI(server.URL).
			PrivateAttributeNames("name").
			CreateEventProcessor(basicClientContext())
		require.NoError(t, err)

		ef := ldevents.NewEventFactory(false, nil)
		ie := ef.NewIdentifyEvent(ldevents.User(lduser.NewUserBuilder("user-key").Email("user-email").Name("user-name").Build()))
		ep.RecordIdentifyEvent(ie)
		ep.Flush()

		r := <-requestsCh
		var jsonData ldvalue.Value
		_ = json.Unmarshal(r.Body, &jsonData)
		assert.Equal(t, 1, jsonData.Count())
		event := jsonData.GetByIndex(0)
		assert.Equal(t, ldvalue.String("identify"), event.GetByKey("kind"))
		assert.Equal(t, ldvalue.String("user-key"), event.GetByKey("user").GetByKey("key"))
		assert.Equal(t, ldvalue.String("user-email"), event.GetByKey("user").GetByKey("email"))
		assert.Equal(t, ldvalue.Null(), event.GetByKey("user").GetByKey("name"))
		assert.Equal(t, ldvalue.ArrayOf(ldvalue.String("name")), event.GetByKey("user").GetByKey("privateAttrs"))
	})
}
