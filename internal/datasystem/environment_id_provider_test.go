package datasystem

import (
	"fmt"
	"sync"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/stretchr/testify/assert"
)

func TestEnvironmentIDProviderConcurrentAccess(t *testing.T) {
	const iterations = 1000
	provider := &environmentIDProvider{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range iterations {
			provider.SetEnvironmentID(ldvalue.NewOptionalString(fmt.Sprintf("env-%d", i)))
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			_ = provider.GetEnvironmentID()
		}
	}()
	wg.Wait()
	assert.Equal(t, ldvalue.NewOptionalString(fmt.Sprintf("env-%d", iterations-1)), provider.GetEnvironmentID())
}
