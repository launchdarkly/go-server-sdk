package ldclient

import (
	"testing"
	"time"
)

func TestParseNilDate(t *testing.T) {
	_, err := parseTime(nil)
	if err == nil {
		t.Errorf("Didn't get expected error when parsing nil date")
	}
}

func TestParseDateZero(t *testing.T) {
	expectedTimeStamp := "1970-01-01T00:00:00Z"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParse(t, expected, expected)
	testParse(t, 0, expected)
	testParse(t, 0.0, expected)
	testParse(t, "0", expected)
	testParse(t, "0.0", expected)
	testParse(t, expectedTimeStamp, expected)
}

func TestParseUtcTimestamp(t *testing.T) {
	expectedTimeStamp := "2016-04-16T22:57:31.684Z"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParse(t, expected, expected)
	testParse(t, 1460847451684, expected)
	testParse(t, 1460847451684.0, expected)
	testParse(t, "1460847451684", expected)
	testParse(t, "1460847451684.0", expected)
	testParse(t, expectedTimeStamp, expected)
}

func TestParseTimezone(t *testing.T) {
	expectedTimeStamp := "2016-04-16T17:09:12.759-07:00"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParse(t, expected, expected)
	testParse(t, 1460851752759, expected)
	testParse(t, 1460851752759.0, expected)
	testParse(t, "1460851752759", expected)
	testParse(t, "1460851752759.0", expected)
	testParse(t, expectedTimeStamp, expected)
}

func TestParseTimezoneNoMillis(t *testing.T) {
	expectedTimeStamp := "2016-04-16T17:09:12-07:00"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParse(t, expected, expected)
	testParse(t, 1460851752000, expected)
	testParse(t, 1460851752000.0, expected)
	testParse(t, "1460851752000", expected)
	testParse(t, "1460851752000.0", expected)
	testParse(t, expectedTimeStamp, expected)
}

func TestParseTimestampBeforeEpoch(t *testing.T) {
	expectedTimeStamp := "1969-12-31T23:57:56.544-00:00"
	expected, _ := time.Parse(time.RFC3339Nano, expectedTimeStamp)
	testParse(t, expected, expected)
	testParse(t, -123456, expected)
	testParse(t, -123456.0, expected)
	testParse(t, "-123456", expected)
	testParse(t, "-123456.0", expected)
	testParse(t, expectedTimeStamp, expected)
}

func testParse(t *testing.T, input interface{}, expected time.Time) {
	actual, err := parseTime(input)
	if err != nil {
		t.Errorf("Got unexpected error: %+v when parsing: %+v", err, input)
	}

	if !actual.Equal(expected) {
		t.Errorf("Got unexpected result: %+v Expected: %+v when parsing: %+v", actual, expected, input)
	}
}
