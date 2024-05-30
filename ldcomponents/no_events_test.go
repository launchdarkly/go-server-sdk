package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-server-sdk/ldevents/v4"
)

func TestNoEvents(t *testing.T) {
	ep, err := NoEvents().Build(basicClientContext())
	require.NoError(t, err)
	defer ep.Close()
	ef := ldevents.NewEventFactory(false, nil)
	ep.RecordIdentifyEvent(ef.NewIdentifyEventData(ldevents.Context(lduser.NewUser("key")), ldvalue.OptionalInt{}))
	ep.Flush()
}
