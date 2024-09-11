package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

type DataSourceStatusReporter interface {
	UpdateStatus(newState interfaces.DataSourceState, newError interfaces.DataSourceErrorInfo)
}
