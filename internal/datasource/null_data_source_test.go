package datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNullDataSource(t *testing.T) {
	d := NewNullDataSource()
	assert.True(t, d.IsInitialized())

	ch := make(chan struct{})
	d.Start(ch)
	_, ok := <-ch
	assert.False(t, ok)

	assert.Nil(t, d.Close())
}
