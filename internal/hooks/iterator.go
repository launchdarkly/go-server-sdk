package hooks

import (
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

type iterator struct {
	reverse    bool
	cursor     int
	collection []ldhooks.Hook
}

// newIterator creates a new hook iterator which can iterate hooks forward or reverse.
//
// The collection being iterated should not be modified during iteration.
//
// Example:
// it := newIterator(false, hooks)
//
//	for it.hasNext() {
//	  hook := it.getNext()
//	}
func newIterator(reverse bool, hooks []ldhooks.Hook) *iterator {
	cursor := -1
	if reverse {
		cursor = len(hooks)
	}
	return &iterator{
		reverse:    reverse,
		cursor:     cursor,
		collection: hooks,
	}
}

func (it *iterator) hasNext() bool {
	nextCursor := it.getNextIndex()
	return it.inBounds(nextCursor)
}

func (it *iterator) inBounds(nextCursor int) bool {
	inBounds := nextCursor < len(it.collection) && nextCursor >= 0
	return inBounds
}

func (it *iterator) getNextIndex() int {
	var nextCursor int
	if it.reverse {
		nextCursor = it.cursor - 1
	} else {
		nextCursor = it.cursor + 1
	}
	return nextCursor
}

func (it *iterator) getNext() (int, ldhooks.Hook) {
	i := it.getNextIndex()
	if it.inBounds(i) {
		it.cursor = i
		return it.cursor, it.collection[it.cursor]
	}
	return it.cursor, nil
}
