package ldevents

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLRUCache(t *testing.T) {
	t.Run("add returns false for never-seen value", func(t *testing.T) {
		cache := newLruCache(10)
		assert.False(t, cache.add("a"))
	})

	t.Run("add returns true for already-seen value", func(t *testing.T) {
		cache := newLruCache(10)
		cache.add("a")
		assert.True(t, cache.add("a"))
	})

	t.Run("oldest value is discarded when capacity is exceeded", func(t *testing.T) {
		cache := newLruCache(2)
		cache.add("a")
		cache.add("b")
		cache.add("c")
		assert.True(t, cache.add("c"))
		assert.True(t, cache.add("b"))
		assert.False(t, cache.add("a"))
	})

	t.Run("re-adding an existing value makes it new again", func(t *testing.T) {
		cache := newLruCache(2)
		cache.add("a")
		cache.add("b")
		cache.add("a")
		cache.add("c")
		assert.True(t, cache.add("c"))
		assert.True(t, cache.add("a"))
		assert.False(t, cache.add("b"))
	})

	t.Run("zero-length cache treats values as new", func(t *testing.T) {
		cache := newLruCache(0)
		assert.False(t, cache.add("a"))
		assert.False(t, cache.add("a"))
	})
}
