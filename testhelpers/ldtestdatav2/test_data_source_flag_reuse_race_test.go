//go:build race

package ldtestdatav2

import (
	"sync"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	eval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

type nilProvider struct{}

func (nilProvider) GetFeatureFlag(string) *ldmodel.FeatureFlag { return nil }
func (nilProvider) GetSegment(string) *ldmodel.Segment         { return nil }

// TestFlagCreateEvalRace verifies that a test flag
// can be created while an existing test flag is being evaluated.
func TestFlagCreateEvalRace(t *testing.T) {
	f := DataSource().Flag("flag").
		IfMatch("is", ldvalue.Bool(true)).
		ThenReturn(true)
	published := f.createFlag(1)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// An in-flight evaluation of the published flag, on the SDK's own goroutine.
		evaluator := eval.NewEvaluator(nilProvider{})
		evaluator.Evaluate(&published, ldcontext.New("user"), nil)
	}()
	// Caller reuses the builder; Builder use must not alter existing flags.
	f.createFlag(2)
	wg.Wait()
}
