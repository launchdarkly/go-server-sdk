package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v3/lduser"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v2"
)

func TestNoEvents(t *testing.T) {
	ep, err := NoEvents().CreateEventProcessor(basicClientContext())
	require.NoError(t, err)
	defer ep.Close()
	ef := ldevents.NewEventFactory(false, nil)
	ep.RecordIdentifyEvent(ef.NewIdentifyEvent(ldevents.Context(lduser.NewUser("key"))))
	ep.Flush()
}
