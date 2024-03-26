package hooks

import (
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

type hookIterator struct {
	reverse    bool
	cursor     int
	collection *[]ldhooks.Hook
}

func newHookIterator(reverse bool, hooks *[]ldhooks.Hook) *hookIterator {
	cursor := -1
	if reverse {
		cursor = len(*hooks)
	}
	return &hookIterator{
		reverse:    reverse,
		cursor:     cursor,
		collection: hooks,
	}
}

func (it *hookIterator) hasNext() bool {
	nextCursor := it.getNextIndex()
	return it.inBounds(nextCursor)
}

func (it *hookIterator) inBounds(nextCursor int) bool {
	inBounds := nextCursor < len(*it.collection) && nextCursor >= 0
	return inBounds
}

func (it *hookIterator) getNextIndex() int {
	var nextCursor int
	if it.reverse {
		nextCursor = it.cursor - 1
	} else {
		nextCursor = it.cursor + 1
	}
	return nextCursor
}

func (it *hookIterator) getNext() (int, ldhooks.Hook) {
	i := it.getNextIndex()
	if it.inBounds(i) {
		it.cursor = i
		return it.cursor, (*it.collection)[it.cursor]
	}
	return it.cursor, nil
}
