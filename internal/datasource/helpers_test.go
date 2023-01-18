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

type filterTestCase struct {
	name   string
	params filterTest
}

type filterTest struct {
	key   string
	query string
}

func filterTests() []filterTestCase {
	return []filterTestCase{
		{"no filter", filterTest{"", ""}},
		{"filter", filterTest{"microservice-1", "filter=microservice-1"}},
		{"filter requires urlencoding", filterTest{"micro service 1", "filter=micro+service+1"}},
	}
}

func testWithFilters(t *testing.T, testFn func(t *testing.T, filterTest filterTest)) {
	for _, test := range filterTests() {
		t.Run(test.name, func(t *testing.T) {
			testFn(t, test.params)
		})
	}
}
