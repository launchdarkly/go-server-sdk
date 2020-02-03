package ldclient

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

const dateStr1 = "2017-12-06T00:00:00.000-07:00"
const dateStr2 = "2017-12-06T00:01:01.000-07:00"
const dateMs1 = 10000000
const dateMs2 = 10000001
const invalidDate = "hey what's this?"

type opTestInfo struct {
	opName      operator
	userValue   interface{}
	clauseValue interface{}
	expected    bool
}

var operatorTests = []opTestInfo{
	// numeric operators
	{"in", int(99), int(99), true},
	{"in", float64(99.0001), float64(99.0001), true},
	{"lessThan", int(1), float64(1.99999), true},
	{"lessThan", float64(1.99999), int(1), false},
	{"lessThan", int(1), uint(2), true},
	{"lessThanOrEqual", int(1), float64(1), true},
	{"greaterThan", int(2), float64(1.99999), true},
	{"greaterThan", float64(1.99999), int(2), false},
	{"greaterThan", int(2), uint(1), true},
	{"greaterThanOrEqual", int(1), float64(1), true},

	// string operators
	{"in", "x", "x", true},
	{"in", "x", "xyz", false},
	{"startsWith", "xyz", "x", true},
	{"startsWith", "x", "xyz", false},
	{"endsWith", "xyz", "z", true},
	{"endsWith", "z", "xyz", false},
	{"contains", "xyz", "y", true},
	{"contains", "y", "xyz", false},

	// mixed strings and numbers
	{"in", "99", int(99), false},
	{"in", int(99), "99", false},
	{"contains", "99", int(99), false},
	{"startsWith", "99", int(99), false},
	{"endsWith", "99", int(99), false},
	{"lessThanOrEqual", "99", int(99), false},
	{"lessThanOrEqual", int(99), "99", false},
	{"greaterThanOrEqual", "99", int(99), false},
	{"greaterThanOrEqual", int(99), "99", false},

	// regex
	{"matches", "hello world", "hello.*rld", true},
	{"matches", "hello world", "hello.*orl", true},
	{"matches", "hello world", "l+", true},
	{"matches", "hello world", "(world|planet)", true},
	{"matches", "hello world", "aloha", false},
	{"matches", "hello world", "***bad regex", false},

	// date operators
	{"before", dateStr1, dateStr2, true},
	{"before", dateMs1, dateMs2, true},
	{"before", dateStr2, dateStr1, false},
	{"before", dateMs2, dateMs1, false},
	{"before", dateStr1, dateStr1, false},
	{"before", dateMs1, dateMs1, false},
	{"before", nil, dateStr1, false},
	{"before", dateStr1, invalidDate, false},
	{"after", dateStr2, dateStr1, true},
	{"after", dateMs2, dateMs1, true},
	{"after", dateStr1, dateStr2, false},
	{"after", dateMs1, dateMs2, false},
	{"after", dateStr1, dateStr1, false},
	{"after", dateMs1, dateMs1, false},
	{"after", nil, dateStr1, false},
	{"after", dateStr1, invalidDate, false},

	// semver operators
	{"semVerEqual", "2.0.0", "2.0.0", true},
	{"semVerEqual", "2.0", "2.0.0", true},
	{"semVerEqual", "2-rc1", "2.0.0-rc1", true},
	{"semVerEqual", "2+build2", "2.0.0+build2", true},
	{"semVerEqual", "2.0.0", "2.0.1", false},
	{"semVerLessThan", "2.0.0", "2.0.1", true},
	{"semVerLessThan", "2.0", "2.0.1", true},
	{"semVerLessThan", "2.0.1", "2.0.0", false},
	{"semVerLessThan", "2.0.1", "2.0", false},
	{"semVerLessThan", "2.0.1", "xbad%ver", false},
	{"semVerLessThan", "2.0.0-rc", "2.0.0-rc.beta", true},
	{"semVerGreaterThan", "2.0.1", "2.0", true},
	{"semVerGreaterThan", "2.0.1", "2.0", true},
	{"semVerGreaterThan", "2.0.0", "2.0.1", false},
	{"semVerGreaterThan", "2.0", "2.0.1", false},
	{"semVerGreaterThan", "2.0.1", "xbad%ver", false},
	{"semVerGreaterThan", "2.0.0-rc.1", "2.0.0-rc.0", true},
}

func TestAllOperators(t *testing.T) {
	for _, ti := range operatorTests {
		t.Run(fmt.Sprintf("%v %s %v should be %v", ti.userValue, ti.opName, ti.clauseValue, ti.expected), func(t *testing.T) {
			uValue := ldvalue.CopyArbitraryValue(ti.userValue)
			cValue := ldvalue.CopyArbitraryValue(ti.clauseValue)
			assert.Equal(t, ti.expected, operatorFn(ti.opName)(uValue, cValue))
		})
	}
}

func TestParseDateZero(t *testing.T) {
	expectedTimeStamp := "1970-01-01T00:00:00Z"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParseTime(t, expected, expected)
	testParseTime(t, 0, expected)
	testParseTime(t, 0.0, expected)
	testParseTime(t, expectedTimeStamp, expected)
}

func TestParseUtcTimestamp(t *testing.T) {
	expectedTimeStamp := "2016-04-16T22:57:31.684Z"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParseTime(t, expected, expected)
	testParseTime(t, 1460847451684, expected)
	testParseTime(t, 1460847451684.0, expected)
	testParseTime(t, expectedTimeStamp, expected)
}

func TestParseTimezone(t *testing.T) {
	expectedTimeStamp := "2016-04-16T17:09:12.759-07:00"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParseTime(t, expected, expected)
	testParseTime(t, 1460851752759, expected)
	testParseTime(t, 1460851752759.0, expected)
	testParseTime(t, expectedTimeStamp, expected)
}

func TestParseTimezoneNoMillis(t *testing.T) {
	expectedTimeStamp := "2016-04-16T17:09:12-07:00"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParseTime(t, expected, expected)
	testParseTime(t, 1460851752000, expected)
	testParseTime(t, 1460851752000.0, expected)
	testParseTime(t, expectedTimeStamp, expected)
}

func TestParseTimestampBeforeEpoch(t *testing.T) {
	expectedTimeStamp := "1969-12-31T23:57:56.544-00:00"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParseTime(t, expected, expected)
	testParseTime(t, -123456, expected)
	testParseTime(t, -123456.0, expected)
	testParseTime(t, expectedTimeStamp, expected)
}

func testParseTime(t *testing.T, input interface{}, expected time.Time) {
	value := ldvalue.CopyArbitraryValue(input)
	actual, ok := parseDateTime(value)
	if !ok {
		t.Errorf("failed to parse: %s", value)
		return
	}

	if !actual.Equal(expected) {
		t.Errorf("Got unexpected result: %+v Expected: %+v when parsing: %s", actual, expected, value)
	}
}
