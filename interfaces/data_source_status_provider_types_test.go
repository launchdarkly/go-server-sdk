package interfaces

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDataSourceStatusProviderTypes(t *testing.T) {
	t.Run("status string representation", func(t *testing.T) {
		now := time.Now()

		s1 := DataSourceStatus{State: DataSourceStateValid, StateSince: now}
		assert.Equal(t, "Status(VALID,"+now.Format(time.RFC3339)+",)", s1.String())

		e := DataSourceErrorInfo{Kind: DataSourceErrorKindErrorResponse, StatusCode: 401, Time: time.Now()}
		s2 := DataSourceStatus{State: DataSourceStateInterrupted, StateSince: now, LastError: e}
		assert.Equal(t, "Status(INTERRUPTED,"+now.Format(time.RFC3339)+","+e.String()+")", s2.String())
	})

	t.Run("error string representation", func(t *testing.T) {
		now := time.Now()

		e1 := DataSourceErrorInfo{Kind: DataSourceErrorKindErrorResponse, StatusCode: 401, Time: time.Now()}
		assert.Equal(t, "ERROR_RESPONSE(401)@"+now.Format(time.RFC3339), e1.String())

		e2 := DataSourceErrorInfo{Kind: DataSourceErrorKindErrorResponse, StatusCode: 401,
			Message: "nope", Time: time.Now()}
		assert.Equal(t, "ERROR_RESPONSE(401,nope)@"+now.Format(time.RFC3339), e2.String())

		e3 := DataSourceErrorInfo{Kind: DataSourceErrorKindNetworkError,
			Message: "nope", Time: time.Now()}
		assert.Equal(t, "NETWORK_ERROR(nope)@"+now.Format(time.RFC3339), e3.String())

		e4 := DataSourceErrorInfo{Kind: DataSourceErrorKindStoreError, Time: time.Now()}
		assert.Equal(t, "STORE_ERROR@"+now.Format(time.RFC3339), e4.String())

		e5 := DataSourceErrorInfo{Kind: DataSourceErrorKindUnknown, Time: time.Now()}
		assert.Equal(t, "UNKNOWN@"+now.Format(time.RFC3339), e5.String())

		e6 := DataSourceErrorInfo{Kind: DataSourceErrorKindUnknown}
		assert.Equal(t, "UNKNOWN", e6.String())
	})
}
