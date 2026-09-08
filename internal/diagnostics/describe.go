// Package diagnostics contains helpers that collect the configuration data for diagnostic events.
package diagnostics

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// descriptionWithoutContext is the shape that components used before version 6.0.0 of the SDK.
// Version 6.0.0 merged two interfaces into subsystems.DiagnosticDescription and gave the method a
// context parameter. A component whose description does not depend on the context may still use
// this shape. The persistent store integrations do.
type descriptionWithoutContext interface {
	DescribeConfiguration() ldvalue.Value
}

// DescribeConfiguration returns the component's own description of its configuration. It returns
// ldvalue.Null() if the component does not describe itself.
//
// A component may declare DescribeConfiguration with or without the client context parameter. Go
// does not allow both on one type, so the two shapes are mutually exclusive.
func DescribeConfiguration(component interface{}, context subsystems.ClientContext) ldvalue.Value {
	if dd, ok := component.(subsystems.DiagnosticDescription); ok {
		return dd.DescribeConfiguration(context)
	}
	if dd, ok := component.(descriptionWithoutContext); ok {
		return dd.DescribeConfiguration()
	}
	return ldvalue.Null()
}
