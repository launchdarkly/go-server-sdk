package datasource

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPStatusError(t *testing.T) {
	var error = httpStatusError{Message: "message", Code: 500}
	assert.Equal(t, "message", error.Error())
}

func TestIsHTTPErrorRecoverable(t *testing.T) {
	for i := 400; i < 500; i++ {
		assert.Equal(t, i == 400 || i == 408 || i == 429, isHTTPErrorRecoverable(i), strconv.Itoa(i))
	}
	for i := 500; i < 600; i++ {
		assert.True(t, isHTTPErrorRecoverable(i))
	}
}

func TestHTTPErrorDescription(t *testing.T) {
	assert.Equal(t, "HTTP error 400", httpErrorDescription(400))
	assert.Equal(t, "HTTP error 401 (invalid SDK key)", httpErrorDescription(401))
	assert.Equal(t, "HTTP error 403 (invalid SDK key)", httpErrorDescription(403))
	assert.Equal(t, "HTTP error 500", httpErrorDescription(500))
}

// filterTest represents the expected URL query parameter that should
// be generated for a particular filter key. For example, filter 'foo' should generate
// query parameter 'filter=foo'.
type filterTest struct {
	key   string
	query string
}

// testWithFilters generates a nested test for a set of relevant filters.
// The 'test' function is executed with the requested filter, and the expected query parameter
// for that filter.
func testWithFilters(t *testing.T, test func(t *testing.T, filterTest filterTest)) {
	testCases := map[string]filterTest{
		"no filter":                   {"", ""},
		"filter requires no encoding": {"microservice-1", "filter=microservice-1"},
		"filter requires urlencoding": {"micro service 1", "filter=micro+service+1"},
	}
	for name, params := range testCases {
		t.Run(name, func(t *testing.T) {
			test(t, params)
		})
	}
}
