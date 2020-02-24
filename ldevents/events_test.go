package ldevents

import "gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"

var defaultEventFactory = NewEventFactory(false, nil)

var noReason = ldreason.EvaluationReason{}
