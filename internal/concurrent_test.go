package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicBoolean(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		var b AtomicBoolean
		assert.False(t, b.Get())
	})

	t.Run("Set", func(t *testing.T) {
		var b AtomicBoolean
		b.Set(true)
		assert.True(t, b.Get())
		b.Set(false)
		assert.False(t, b.Get())
	})

	t.Run("GetAndSet", func(t *testing.T) {
		var b AtomicBoolean
		assert.False(t, b.GetAndSet(true))
		assert.True(t, b.Get())
		assert.True(t, b.GetAndSet(false))
		assert.False(t, b.Get())
	})

	t.Run("data race", func(t *testing.T) {
		// should be flagged by race detector if our implementation is unsafer
		done := make(chan struct{})
		var b AtomicBoolean
		go func() {
			for i := 0; i < 100; i++ {
				b.Set(true)
			}
			done <- struct{}{}
		}()
		go func() {
			for i := 0; i < 100; i++ {
				b.Get()
			}
			done <- struct{}{}
		}()
		<-done
		<-done
	})
}
