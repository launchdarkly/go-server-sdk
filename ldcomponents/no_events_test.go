package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"
)

func TestNoEvents(t *testing.T) {
	ep, err := NoEvents().CreateEventProcessor(basicClientContext())
	require.NoError(t, err)
	defer ep.Close()
	ef := ldevents.NewEventFactory(false, nil)
	ep.SendEvent(ef.NewIdentifyEvent(ldevents.User(lduser.NewUser("key"))))
	ep.Flush()
}
