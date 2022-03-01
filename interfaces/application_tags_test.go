package interfaces

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplicationTags(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		tags, allValid := NewApplicationTags(map[string][]string{})
		assert.True(t, allValid)
		assert.Equal(t, ApplicationTags{}, tags)
	})

	t.Run("all strings valid", func(t *testing.T) {
		tags, allValid := NewApplicationTags(map[string][]string{
			"tag2": {"value2a", "value2b"},
			"tag1": {"value1"},
		})
		assert.True(t, allValid)
		assert.Equal(t, ApplicationTags{
			all: map[string][]string{
				"tag2": {"value2a", "value2b"},
				"tag1": {"value1"},
			},
		}, tags)
	})

	t.Run("invalid key", func(t *testing.T) {
		tags, allValid := NewApplicationTags(map[string][]string{
			"tag2!!": {"value2a", "value2b"},
			"tag1":   {"value1"},
		})
		assert.False(t, allValid)
		assert.Equal(t, ApplicationTags{
			all: map[string][]string{
				"tag1": {"value1"},
			},
		}, tags)
	})

	t.Run("invalid value", func(t *testing.T) {
		tags, allValid := NewApplicationTags(map[string][]string{
			"tag2": {"value2a!!", "value2b"},
			"tag1": {"value1"},
		})
		assert.False(t, allValid)
		assert.Equal(t, ApplicationTags{
			all: map[string][]string{
				"tag2": {"value2b"},
				"tag1": {"value1"},
			},
		}, tags)
	})

	t.Run("get keys and values", func(t *testing.T) {
		tags, _ := NewApplicationTags(map[string][]string{
			"tag2": {"value2a", "value2b"},
			"tag1": {"value1"},
		})
		keys := tags.Keys()
		sort.Strings(keys)
		assert.Equal(t, []string{"tag1", "tag2"}, keys)
		assert.Equal(t, []string{"value1"}, tags.Values("tag1"))
		assert.Equal(t, []string{"value2a", "value2b"}, tags.Values("tag2"))
	})
}
