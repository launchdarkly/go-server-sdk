package ldredis

import (
	"testing"

	r "github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/assert"
)

func TestDataSourceBuilder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		b := DataStore()
		assert.Len(t, b.dialOptions, 0)
		assert.Nil(t, b.pool)
		assert.Equal(t, DefaultPrefix, b.prefix)
		assert.Equal(t, DefaultURL, b.url)
	})

	t.Run("DialOptions", func(t *testing.T) {
		o1 := r.DialPassword("p")
		o2 := r.DialTLSSkipVerify(true)
		b := DataStore().DialOptions(o1, o2)
		assert.Len(t, b.dialOptions, 2) // a DialOption is a function, so can't do an equality test
	})

	t.Run("HostAndPort", func(t *testing.T) {
		b := DataStore().HostAndPort("mine", 4000)
		assert.Equal(t, "redis://mine:4000", b.url)
	})

	t.Run("Pool", func(t *testing.T) {
		p := &r.Pool{MaxActive: 999}
		b := DataStore().Pool(p)
		assert.Equal(t, p, b.pool)
	})

	t.Run("Prefix", func(t *testing.T) {
		b := DataStore().Prefix("p")
		assert.Equal(t, "p", b.prefix)

		b.Prefix("")
		assert.Equal(t, DefaultPrefix, b.prefix)
	})

	t.Run("URL", func(t *testing.T) {
		url := "redis://mine"
		b := DataStore().URL(url)
		assert.Equal(t, url, b.url)

		b.URL("")
		assert.Equal(t, DefaultURL, b.url)
	})
}
