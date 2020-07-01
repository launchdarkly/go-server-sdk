package internal

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
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
	tracker := NewFlagTrackerImpl(broadcaster, func(flag string, user lduser.User, defaultValue ldvalue.Value) ldvalue.Value {
		resultLock.Lock()
		defer resultLock.Unlock()
		return resultMap[user.GetKey()]
	})

	resultMap[user.GetKey()] = ldvalue.Bool(false)
	resultMap[otherUser.GetKey()] = ldvalue.Bool(false)

	ch1 := tracker.AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
	ch2 := tracker.AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
	ch3 := tracker.AddFlagValueChangeListener(flagKey, otherUser, ldvalue.Null())
	tracker.RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works

	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch1)
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch2)
	sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch3)

	// make the flag true for the first user only, and broadcast a flag change event
	resultLock.Lock()
	resultMap[user.GetKey()] = ldvalue.Bool(true)
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
