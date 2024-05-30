package ldevents

import (
	"encoding/json"
	"time"

	"github.com/launchdarkly/go-jsonstream/v3/jwriter"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const (
	testContextKey = "userKey"

	sdkKey = "SDK_KEY"

	fakeBaseURI       = "https://fake-server"
	fakeEventsURI     = fakeBaseURI + "/bulk"
	fakeDiagnosticURI = fakeBaseURI + "/diagnostic"

	fakeTime = ldtime.UnixMillisecondTime(100000)

	briefRetryDelay = 50 * time.Millisecond
)

var (
	testValue                   = ldvalue.String("value")
	testEvalDetailWithoutReason = ldreason.NewEvaluationDetail(testValue, 2, noReason)
	undefInt                    = ldvalue.OptionalInt{}
	arbitraryJSONData           = []byte(`"hello"`)
)

func fakeTimeFn() ldtime.UnixMillisecondTime { return fakeTime }

func basicContext() EventInputContext {
	return Context(ldcontext.NewBuilder(testContextKey).Name("Red").Build())
}

func basicConfigWithoutPrivateAttrs() EventsConfiguration {
	return EventsConfiguration{
		Capacity:              1000,
		FlushInterval:         1 * time.Hour,
		UserKeysCapacity:      1000,
		UserKeysFlushInterval: 1 * time.Hour,
	}
}

func contextJSON(c EventInputContext, config EventsConfiguration) json.RawMessage {
	formatter := newEventContextFormatter(config)
	w := jwriter.NewWriter()
	formatter.WriteContext(&w, &c)
	if err := w.Error(); err != nil {
		panic(err)
	}
	return w.Bytes()
}
