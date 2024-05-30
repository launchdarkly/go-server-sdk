package ldevents

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/require"
)

func TestNullEventProcessor(t *testing.T) {
	// Just verifies that these methods don't panic
	n := NewNullEventProcessor()
	n.RecordEvaluation(defaultEventFactory.NewUnknownFlagEvaluationData("x", basicContext(), ldvalue.Null(),
		ldreason.EvaluationReason{}))
	n.RecordIdentifyEvent(defaultEventFactory.NewIdentifyEventData(basicContext(), ldvalue.OptionalInt{}))
	n.RecordMigrationOpEvent(MigrationOpEventData{})
	n.RecordCustomEvent(defaultEventFactory.NewCustomEventData("x", basicContext(), ldvalue.Null(), false, 0, ldvalue.OptionalInt{}))
	n.RecordRawEvent([]byte("{}"))
	n.Flush()
	n.FlushBlocking(0)

	require.NoError(t, n.Close())
}
