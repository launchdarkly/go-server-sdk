package internal

import (
	"sync"
	"testing"

	intf "gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/sharedtest"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

func TestFlagChangeListeners(t *testing.T) {
	flagKey := "flagkey"

	broadcaster := NewFlagChangeEventBroadcaster()
	defer broadcaster.Close()
	tracker := NewFlagTrackerImpl(broadcaster, nil)

	ch1 := tracker.AddFlagChangeListener()
	ch2 := tracker.AddFlagChangeListener()

	broadcaster.Broadcast(intf.FlagChangeEvent{Key: flagKey})

	sharedtest.ExpectFlagChangeEvents(t, ch1, flagKey)
	sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)

	tracker.RemoveFlagChangeListener(ch1)

	broadcaster.Broadcast(intf.FlagChangeEvent{Key: flagKey})

	sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)
	sharedtest.ExpectNoMoreFlagChangeEvents(t, ch1)
}

func TestFlagValueChangeListener(t *testing.T) {
	flagKey := "important-flag"
	user := lduser.NewUser("important-user")
	otherUser := lduser.NewUser("unimportant-user")
	resultMap := make(map[string]ldvalue.Value)
	resultLock := sync.Mutex{}

	broadcaster := NewFlagChangeEventBroadcaster()
	defer broadcaster.Close()
	tracker := NewFlagTrackerImpl(broadcaster, func(flag string, user ldcontext.Context, defaultValue ldvalue.Value) ldvalue.Value {
		resultLock.Lock()
		defer resultLock.Unlock()
		return resultMap[user.Key()]
	})

	resultMap[user.Key()] = ldvalue.Bool(false)
	resultMap[otherUser.Key()] = ldvalue.Bool(false)

	ch1 := tracker.AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
	ch2 := tracker.AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
	ch3 := tracker.AddFlagValueChangeListener(flagKey, otherUser, ldvalue.Null())
	tracker.RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works

	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch1)
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch2)
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch3)

	// make the flag true for the first user only, and broadcast a flag change event
	resultLock.Lock()
	resultMap[user.Key()] = ldvalue.Bool(true)
	resultLock.Unlock()
	broadcaster.Broadcast(intf.FlagChangeEvent{Key: flagKey})

	// ch1 receives a value change event
	event1 := <-ch1
	assert.Equal(t, flagKey, event1.Key)
	assert.Equal(t, ldvalue.Bool(false), event1.OldValue)
	assert.Equal(t, ldvalue.Bool(true), event1.NewValue)

	// ch2 doesn't receive one, because it was unregistered
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch2)

	// ch3 doesn't receive one, because the flag's value hasn't changed for otherUser
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch3)

	// broadcast a flag change event for a different flag
	broadcaster.Broadcast(intf.FlagChangeEvent{Key: "other-flag"})
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch1)
}
