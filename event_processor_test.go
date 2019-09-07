package ldclient

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var BuiltinAttributes = []string{
	"avatar",
	"country",
	"email",
	"firstName",
	"ip",
	"lastName",
	"name",
	"secondary",
}

var epDefaultConfig = Config{
	SendEvents:            true,
	Capacity:              1000,
	FlushInterval:         1 * time.Hour,
	UserKeysCapacity:      1000,
	UserKeysFlushInterval: 1 * time.Hour,
}

var epDefaultUser = User{
	Key:  strPtr("userKey"),
	Name: strPtr("Red"),
}

var userJson = map[string]interface{}{"key": "userKey", "name": "Red"}
var filteredUserJson = map[string]interface{}{"key": "userKey", "privateAttrs": []interface{}{"name"}}

const (
	sdkKey = "SDK_KEY"
)

type stubTransport struct {
	messageSent chan *http.Request
	statusCode  int
	serverTime  uint64
	error       error
}

var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func init() {
	sort.Strings(BuiltinAttributes)
}

func TestIdentifyEventIsQueued(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 1, len(output)) {
		assertIdentifyEventMatches(t, ie, userJson, output[0])
	}
}

func TestUserDetailsAreScrubbedInIdentifyEvent(t *testing.T) {
	config := epDefaultConfig
	config.AllAttributesPrivate = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 1, len(output)) {
		assertIdentifyEventMatches(t, ie, filteredUserJson, output[0])
	}
}

func TestFeatureEventIsSummarizedAndNotTrackedByDefault(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	flag := FeatureFlag{
		Key:     "flagkey",
		Version: 11,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[1])
	}
}

func TestIndividualFeatureEventIsQueuedWhenTrackEventsIsTrue(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	flag := FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 3, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0])
		assertFeatureEventMatches(t, fe, flag, value, false, nil, output[1])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[2])
	}
}

func TestUserDetailsAreScrubbedInIndexEvent(t *testing.T) {
	config := epDefaultConfig
	config.AllAttributesPrivate = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	flag := FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 3, len(output)) {
		assertIndexEventMatches(t, fe, filteredUserJson, output[0])
		assertFeatureEventMatches(t, fe, flag, value, false, nil, output[1])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[2])
	}
}

func TestFeatureEventCanContainInlineUser(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	flag := FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertFeatureEventMatches(t, fe, flag, value, false, &userJson, output[0])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[1])
	}
}

func TestUserDetailsAreScrubbedInFeatureEvent(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	config.AllAttributesPrivate = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	flag := FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertFeatureEventMatches(t, fe, flag, value, false, &filteredUserJson, output[0])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[1])
	}
}

func TestFeatureEventCanContainReason(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	flag := FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	fe.Reason.Reason = evalReasonFallthroughInstance
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertFeatureEventMatches(t, fe, flag, value, false, &userJson, output[0])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[1])
	}
}

func TestIndexEventIsGeneratedForNonTrackedFeatureEventEvenIfInliningIsOn(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	flag := FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: false,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0]) // we get this because we are *not* getting the full event
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[1])
	}
}

func TestDebugEventIsAddedIfFlagIsTemporarilyInDebugMode(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	futureTime := now() + 1000000
	flag := FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          false,
		DebugEventsUntilDate: &futureTime,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 3, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0])
		assertFeatureEventMatches(t, fe, flag, value, true, &userJson, output[1])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[2])
	}
}

func TestEventCanBeBothTrackedAndDebugged(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	futureTime := now() + 1000000
	flag := FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          true,
		DebugEventsUntilDate: &futureTime,
	}
	value := "value"
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 4, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0])
		assertFeatureEventMatches(t, fe, flag, value, false, nil, output[1])
		assertFeatureEventMatches(t, fe, flag, value, true, &userJson, output[2])
		assertSummaryEventHasCounter(t, flag, intPtr(2), value, 1, output[3])
	}
}

func TestDebugModeExpiresBasedOnClientTimeIfClienttTimeIsLater(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	// Pick a server time that is somewhat behind the client time
	serverTime := now() - 20000
	st.serverTime = serverTime

	// Send and flush an event we don't care about, just to set the last server time
	ie := NewIdentifyEvent(User{Key: strPtr("otherUser")})
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()
	st.getNextRequest()

	// Now send an event with debug mode on, with a "debug until" time that is further in
	// the future than the server time, but in the past compared to the client.
	debugUntil := serverTime + 1000
	flag := FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          false,
		DebugEventsUntilDate: &debugUntil,
	}
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, nil, nil, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0])
		// should get a summary event only, not a debug event
		assertSummaryEventHasCounter(t, flag, nil, nil, 1, output[1])
	}
}

func TestDebugModeExpiresBasedOnServerTimeIfServerTimeIsLater(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	// Pick a server time that is somewhat ahead of the client time
	serverTime := now() + 20000
	st.serverTime = serverTime

	// Send and flush an event we don't care about, just to set the last server time
	ie := NewIdentifyEvent(User{Key: strPtr("otherUser")})
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()
	st.getNextRequest()

	// Now send an event with debug mode on, with a "debug until" time that is further in
	// the future than the client time, but in the past compared to the server.
	debugUntil := serverTime - 1000
	flag := FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          false,
		DebugEventsUntilDate: &debugUntil,
	}
	fe := newSuccessfulEvalEvent(&flag, epDefaultUser, nil, nil, nil, nil, false, nil)
	ep.SendEvent(fe)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertIndexEventMatches(t, fe, userJson, output[0])
		// should get a summary event only, not a debug event
		assertSummaryEventHasCounter(t, flag, nil, nil, 1, output[1])
	}
}

func TestTwoFeatureEventsForSameUserGenerateOnlyOneIndexEvent(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	flag1 := FeatureFlag{
		Key:         "flagkey1",
		Version:     11,
		TrackEvents: true,
	}
	flag2 := FeatureFlag{
		Key:         "flagkey2",
		Version:     22,
		TrackEvents: true,
	}
	value := "value"
	fe1 := newSuccessfulEvalEvent(&flag1, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	fe2 := newSuccessfulEvalEvent(&flag2, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	ep.SendEvent(fe1)
	ep.SendEvent(fe2)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 4, len(output)) {
		assertIndexEventMatches(t, fe1, userJson, output[0])
		assertFeatureEventMatches(t, fe1, flag1, value, false, nil, output[1])
		assertFeatureEventMatches(t, fe2, flag2, value, false, nil, output[2])
		assertSummaryEventHasCounter(t, flag1, intPtr(2), value, 1, output[3])
		assertSummaryEventHasCounter(t, flag2, intPtr(2), value, 1, output[3])
	}
}

func TestNonTrackedEventsAreSummarized(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	flag1 := FeatureFlag{
		Key:         "flagkey1",
		Version:     11,
		TrackEvents: false,
	}
	flag2 := FeatureFlag{
		Key:         "flagkey2",
		Version:     22,
		TrackEvents: false,
	}
	value := "value"
	fe1 := newSuccessfulEvalEvent(&flag1, epDefaultUser, intPtr(2), value, nil, nil, false, nil)
	fe2 := newSuccessfulEvalEvent(&flag2, epDefaultUser, intPtr(3), value, nil, nil, false, nil)
	fe3 := newSuccessfulEvalEvent(&flag2, epDefaultUser, intPtr(3), value, nil, nil, false, nil)
	ep.SendEvent(fe1)
	ep.SendEvent(fe2)
	ep.SendEvent(fe3)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertIndexEventMatches(t, fe1, userJson, output[0])

		seo := output[1]
		assertSummaryEventHasCounter(t, flag1, intPtr(2), value, 1, seo)
		assertSummaryEventHasCounter(t, flag2, intPtr(3), value, 2, seo)
		assert.Equal(t, float64(fe1.CreationDate), seo["startDate"])
		assert.Equal(t, float64(fe2.CreationDate), seo["endDate"])
	}
}

func TestCustomEventIsQueuedWithUser(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	data := map[string]interface{}{
		"thing": "stuff",
	}
	ce := NewCustomEvent("eventkey", epDefaultUser, data)
	ep.SendEvent(ce)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 2, len(output)) {
		assertIndexEventMatches(t, ce, userJson, output[0])

		ceo := output[1]
		expected := map[string]interface{}{
			"kind":         "custom",
			"creationDate": float64(ce.CreationDate),
			"key":          ce.Key,
			"data":         data,
			"userKey":      *epDefaultUser.Key,
		}
		assert.Equal(t, expected, ceo)
	}
}

func TestCustomEventCanContainInlineUser(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, st := createEventProcessor(config)
	defer ep.Close()

	data := map[string]interface{}{
		"thing": "stuff",
	}
	ce := NewCustomEvent("eventkey", epDefaultUser, data)
	ep.SendEvent(ce)

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 1, len(output)) {
		ceo := output[0]
		expected := map[string]interface{}{
			"kind":         "custom",
			"creationDate": float64(ce.CreationDate),
			"key":          ce.Key,
			"data":         data,
			"user":         jsonMap(epDefaultUser),
		}
		assert.Equal(t, expected, ceo)
	}
}

func TestClosingEventProcessorForcesSynchronousFlush(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Close()

	output := getEventsFromRequest(st)
	if assert.Equal(t, 1, len(output)) {
		assertIdentifyEventMatches(t, ie, userJson, output[0])
	}
}

func TestNothingIsSentIfThereAreNoEvents(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	ep.Flush()
	ep.waitUntilInactive()

	msg := st.getNextRequest()
	assert.Nil(t, msg)
}

func TestSdkKeyIsSent(t *testing.T) {
	ep, st := createEventProcessor(epDefaultConfig)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	msg := st.getNextRequest()
	assert.Equal(t, sdkKey, msg.Header.Get("Authorization"))
}

func TestUserAgentIsSent(t *testing.T) {
	config := epDefaultConfig
	config.UserAgent = "SecretAgent"
	ep, st := createEventProcessor(config)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	msg := st.getNextRequest()
	assert.Equal(t, config.UserAgent, msg.Header.Get("User-Agent"))
}

func TestDefaultPathIsAddedToEventsUri(t *testing.T) {
	config := epDefaultConfig
	config.EventsUri = "http://fake/"
	ep, st := createEventProcessor(config)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	msg := st.getNextRequest()
	assert.Equal(t, "http://fake/bulk", msg.URL.String())
}

func TestTrailingSlashIsOptionalForEventsUri(t *testing.T) {
	config := epDefaultConfig
	config.EventsUri = "http://fake"
	ep, st := createEventProcessor(config)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	msg := st.getNextRequest()
	assert.Equal(t, "http://fake/bulk", msg.URL.String())
}

func TestDefaultPathIsNotAddedToCustomEndpoint(t *testing.T) {
	config := epDefaultConfig
	config.EventsEndpointUri = "http://fake/"
	ep, st := createEventProcessor(config)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	msg := st.getNextRequest()
	assert.Equal(t, "http://fake/", msg.URL.String())
}

var httpErrorTests = []struct {
	status      int
	recoverable bool
}{
	{400, true},
	{401, false},
	{403, false},
	{408, true},
	{429, true},
	{500, true},
	{503, true},
}

func TestHTTPErrorHandling(t *testing.T) {
	for _, tt := range httpErrorTests {
		t.Run(fmt.Sprintf("%d error, recoverable: %v", tt.status, tt.recoverable), func(t *testing.T) {
			ep, st := createEventProcessor(epDefaultConfig)
			defer ep.Close()

			st.statusCode = tt.status

			ie := NewIdentifyEvent(epDefaultUser)
			ep.SendEvent(ie)
			ep.Flush()
			ep.waitUntilInactive()

			msg := st.getNextRequest()
			assert.NotNil(t, msg)

			if tt.recoverable {
				msg = st.getNextRequest() // 2nd request is a retry of the 1st
				assert.NotNil(t, msg)
				msg = st.getNextRequest()
				assert.Nil(t, msg)
			} else {
				msg = st.getNextRequest()
				assert.Nil(t, msg)

				ep.SendEvent(ie)
				ep.Flush()
				ep.waitUntilInactive()

				msg = st.getNextRequest()
				assert.Nil(t, msg)
			}
		})
	}
}

func TestEventPostingUsesHTTPClientFactory(t *testing.T) {
	postedURLs := make(chan string, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postedURLs <- r.URL.Path
		w.WriteHeader(200)
	}))
	defer ts.Close()
	defer ts.CloseClientConnections()

	cfg := Config{
		Logger:            log.New(ioutil.Discard, "", 0),
		EventsUri:         ts.URL,
		Capacity:          1000,
		HTTPClientFactory: urlAppendingHTTPClientFactory("/transformed"),
	}

	ep := NewDefaultEventProcessor(sdkKey, cfg, nil)
	defer ep.Close()

	ie := NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()

	postedURL := <-postedURLs

	assert.Equal(t, "/bulk/transformed", postedURL)
}

func TestPanicInSerializationOfOneUserDoesNotDropEvents(t *testing.T) {
	user1 := User{
		Key:  strPtr("user1"),
		Name: strPtr("Bandit"),
	}
	user2 := User{
		Key:  strPtr("user2"),
		Name: strPtr("Tinker"),
	}
	errorMessage := "boom"
	user3Custom := map[string]interface{}{
		"uh-oh": valueThatPanicsWhenMarshalledToJSON(errorMessage), // see user_filter_test.go
	}
	user3 := User{
		Key:    strPtr("user3"),
		Name:   strPtr("Pirate"),
		Custom: &user3Custom,
	}

	config := epDefaultConfig
	logger := newMockLogger("")
	config.Loggers.SetBaseLogger(logger)
	ep, st := createEventProcessor(config)
	defer ep.Close()

	ep.SendEvent(NewIdentifyEvent(user1))
	ep.SendEvent(NewIdentifyEvent(user2))
	ep.SendEvent(NewIdentifyEvent(user3))

	output := flushAndGetEvents(ep, st)
	if assert.Equal(t, 3, len(output)) {
		assert.Equal(t, "identify", output[0]["kind"])
		assert.Equal(t, jsonMap(user1), output[0]["user"])

		assert.Equal(t, "identify", output[1]["kind"])
		assert.Equal(t, jsonMap(user2), output[1]["user"])

		partialUser := map[string]interface{}{
			"key":  *user3.Key,
			"name": *user3.Name,
		}
		assert.Equal(t, "identify", output[2]["kind"])
		assert.Equal(t, partialUser, output[2]["user"])
	}

	expectedMessage := "ERROR: " + fmt.Sprintf(userSerializationErrorMessage, describeUserForErrorLog(&user3, false), errorMessage)
	assert.Equal(t, []string{expectedMessage}, logger.output)
}

func jsonMap(o interface{}) map[string]interface{} {
	bytes, _ := json.Marshal(o)
	var result map[string]interface{}
	json.Unmarshal(bytes, &result)
	return result
}

func assertIdentifyEventMatches(t *testing.T, sourceEvent Event, encodedUser map[string]interface{}, output map[string]interface{}) {
	expected := map[string]interface{}{
		"kind":         "identify",
		"key":          *sourceEvent.GetBase().User.Key,
		"creationDate": float64(sourceEvent.GetBase().CreationDate),
		"user":         encodedUser,
	}
	assert.Equal(t, expected, output)
}

func assertIndexEventMatches(t *testing.T, sourceEvent Event, encodedUser map[string]interface{}, output map[string]interface{}) {
	expected := map[string]interface{}{
		"kind":         "index",
		"creationDate": float64(sourceEvent.GetBase().CreationDate),
		"user":         encodedUser,
	}
	assert.Equal(t, expected, output)
}

func assertFeatureEventMatches(t *testing.T, sourceEvent FeatureRequestEvent, flag FeatureFlag,
	value interface{}, debug bool, inlineUser *map[string]interface{}, output map[string]interface{}) {
	kind := "feature"
	if debug {
		kind = "debug"
	}
	expected := map[string]interface{}{
		"kind":         kind,
		"creationDate": float64(sourceEvent.CreationDate),
		"key":          flag.Key,
		"version":      float64(flag.Version),
		"value":        value,
		"default":      nil,
	}
	if sourceEvent.Variation != nil {
		expected["variation"] = float64(*sourceEvent.Variation)
	}
	if sourceEvent.Reason.Reason != nil {
		expected["reason"] = jsonMap(sourceEvent.Reason)
	}
	if inlineUser == nil {
		expected["userKey"] = *sourceEvent.User.Key
	} else {
		expected["user"] = *inlineUser
	}
	assert.Equal(t, expected, output)
}

func assertSummaryEventHasFlag(t *testing.T, flag FeatureFlag, output map[string]interface{}) bool {
	if assert.Equal(t, "summary", output["kind"]) {
		flags, _ := output["features"].(map[string]interface{})
		return assert.NotNil(t, flags) && assert.NotNil(t, flags[flag.Key])
	}
	return false
}

func assertSummaryEventHasCounter(t *testing.T, flag FeatureFlag, variation *int, value interface{}, count int, output map[string]interface{}) {
	if assertSummaryEventHasFlag(t, flag, output) {
		f, _ := output["features"].(map[string]interface{})[flag.Key].(map[string]interface{})
		assert.NotNil(t, f)
		expected := map[string]interface{}{
			"value":   value,
			"count":   float64(count),
			"version": float64(flag.Version),
		}
		if variation != nil {
			expected["variation"] = float64(*variation)
		}
		assert.Contains(t, f["counters"], expected)
	}
}

func createEventProcessor(config Config) (*defaultEventProcessor, *stubTransport) {
	transport := &stubTransport{
		statusCode:  200,
		messageSent: make(chan *http.Request, 100),
	}
	client := &http.Client{
		Transport: transport,
	}
	ep := NewDefaultEventProcessor(sdkKey, config, client)
	return ep.(*defaultEventProcessor), transport
}

func flushAndGetEvents(ep *defaultEventProcessor, st *stubTransport) []map[string]interface{} {
	ep.Flush()
	ep.waitUntilInactive()
	return getEventsFromRequest(st)
}

func getEventsFromRequest(st *stubTransport) (output []map[string]interface{}) {
	msg := st.getNextRequest()
	if msg == nil {
		return
	}
	bytes, err := ioutil.ReadAll(msg.Body)
	if err != nil {
		return
	}
	json.Unmarshal(bytes, &output)
	return
}

func (t *stubTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.messageSent <- request
	if t.error != nil {
		return nil, t.error
	}
	resp := http.Response{
		StatusCode: t.statusCode,
		Header:     make(http.Header),
		Request:    request,
	}
	if t.serverTime != 0 {
		ts := epoch.Add(time.Duration(t.serverTime) * time.Millisecond)
		resp.Header.Add("Date", ts.Format(http.TimeFormat))
	}
	return &resp, nil
}

func (t *stubTransport) getNextRequest() *http.Request {
	select {
	case msg := <-t.messageSent:
		return msg
	default:
		return nil
	}
}

// used only for testing - ensures that all pending messages and flushes have completed
func (ep *defaultEventProcessor) waitUntilInactive() {
	m := syncEventsMessage{replyCh: make(chan struct{})}
	ep.inboxCh <- m
	<-m.replyCh // Now we know that all events prior to this call have been processed
}
