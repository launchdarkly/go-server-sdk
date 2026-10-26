package internal

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/stretchr/testify/assert"
)

func TestEnvironmentIDHolderZeroValueIsUndefined(t *testing.T) {
	var holder EnvironmentIDHolder
	assert.Equal(t, ldvalue.OptionalString{}, holder.Get())
}

func TestEnvironmentIDHolderReturnsLatestValue(t *testing.T) {
	var holder EnvironmentIDHolder
	holder.Set(ldvalue.NewOptionalString("env-1"))
	assert.Equal(t, ldvalue.NewOptionalString("env-1"), holder.Get())

	holder.Set(ldvalue.NewOptionalString("env-2"))
	assert.Equal(t, ldvalue.NewOptionalString("env-2"), holder.Get())

	holder.Set(ldvalue.OptionalString{})
	assert.Equal(t, ldvalue.OptionalString{}, holder.Get())
}

func BenchmarkEnvironmentIDHolderSetUnchangedNoAlloc(b *testing.B) {
	var holder EnvironmentIDHolder
	environmentID := ldvalue.NewOptionalString("env-1")
	holder.Set(environmentID)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		holder.Set(environmentID)
	}
}

func BenchmarkEnvironmentIDHolderGetParallelNoAlloc(b *testing.B) {
	var holder EnvironmentIDHolder
	holder.Set(ldvalue.NewOptionalString("env-1"))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		var environmentID ldvalue.OptionalString
		for pb.Next() {
			environmentID = holder.Get()
		}
		_ = environmentID
	})
}
