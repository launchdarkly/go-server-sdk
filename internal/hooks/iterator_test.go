package hooks

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
)

func TestIterator(t *testing.T) {
	testCases := []bool{false, true}
	for _, reverse := range testCases {
		t.Run(fmt.Sprintf("reverse: %v", reverse), func(t *testing.T) {
			t.Run("empty collection", func(t *testing.T) {

				var hooks []ldhooks.Hook
				it := newIterator(reverse, hooks)

				assert.False(t, it.hasNext())

				_, value := it.getNext()
				assert.Zero(t, value)

			})

			t.Run("collection with items", func(t *testing.T) {
				hooks := []ldhooks.Hook{
					sharedtest.NewTestHook("a"),
					sharedtest.NewTestHook("b"),
					sharedtest.NewTestHook("c"),
				}

				it := newIterator(reverse, hooks)

				var cursor int
				count := 0
				if reverse {
					cursor = 2
				} else {
					cursor += 0
				}
				for it.hasNext() {
					index, value := it.getNext()
					assert.Equal(t, cursor, index)
					assert.Equal(t, hooks[cursor].Metadata().Name(), value.Metadata().Name())

					count += 1

					if reverse {
						cursor -= 1
					} else {
						cursor += 1
					}

				}
				assert.Equal(t, 3, count)
				assert.False(t, it.hasNext())
			})
		})
	}
}
