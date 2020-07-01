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
