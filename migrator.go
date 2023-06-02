package ldclient

import (
	"math/rand"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

// MigrationStage TKTK
type MigrationStage int

// String TKTK
func (s MigrationStage) String() string {
	switch s {
	case Off:
		return "off"
	case DualWrite:
		return "dualwrite"
	case Shadow:
		return "shadow"
	case Live:
		return "live"
	case RampDown:
		return "rampdown"
	case Complete:
		return "complete"
	default:
		return "off"
	}
}

const (
	// Off Stage 1 - migration hasn't started, "old" is authoritative for reads and writes
	Off MigrationStage = iota

	// DualWrite Stage 2 - write to both "old" and "new", "old" is authoritative for reads
	DualWrite

	// Shadow Stage 3 - both "new" and "old" versions run with a preference for "old"
	Shadow

	// Live Stage 4 - both "new" and "old" versions run with a preference for "new"
	Live

	// RampDown Stage 5 only read from "new", write to "old" and "new"
	RampDown

	// Complete Stage 6 - migration is done
	Complete
)

// MigrationComparisonFn TKTK
type MigrationComparisonFn func(interface{}, interface{}) bool

// MigrationImplFn TKTK
type MigrationImplFn func() (interface{}, error)

// MigrationConfig TKTK
type migrationConfig struct {
	old     MigrationImplFn
	new     MigrationImplFn
	compare MigrationComparisonFn
}

// Migrator TKTK
type Migrator interface {
	ValidateRead(key string, context ldcontext.Context, defaultStage MigrationStage) (interface{}, error)
	ValidateWrite(key string, context ldcontext.Context, defaultStage MigrationStage) (interface{}, error, error)
}

type migratorImpl struct {
	client                *LDClient
	randomizeSeqExecOrder bool

	readConfig  migrationConfig
	writeConfig migrationConfig

	oldLatencyFn func(key string, context ldcontext.Context, elapsed time.Duration) error
	newLatencyFn func(key string, context ldcontext.Context, elapsed time.Duration) error
}

func (m migratorImpl) ValidateRead(
	key string, context ldcontext.Context, defaultStage MigrationStage,
) (interface{}, error) {
	stage, err := m.client.MigrationVariation(key, context, defaultStage)
	if err != nil {
		return nil, err
	}

	// Reads should always be available to run in parallel.
	runInParallel := true

	oldExecutor := &migrationExecutor{
		label:     "old",
		key:       key,
		impl:      m.readConfig.old,
		latencyFn: m.oldLatencyFn,
	}
	newExecutor := &migrationExecutor{
		label:     "new",
		key:       key,
		impl:      m.readConfig.new,
		latencyFn: m.newLatencyFn,
	}

	switch stage {
	case Off:
		return oldExecutor.exec(context)
	case DualWrite:
		return oldExecutor.exec(context)
	case Shadow:
		result, activeErr, _ := m.runBoth(key, context, *oldExecutor, *newExecutor, m.readConfig.compare, runInParallel)
		return result, activeErr
	case Live:
		result, activeErr, _ := m.runBoth(key, context, *newExecutor, *oldExecutor, m.readConfig.compare, runInParallel)
		return result, activeErr
	case RampDown:
		return newExecutor.exec(context)
	case Complete:
		return newExecutor.exec(context)
	}

	return nil, nil
}

func (m migratorImpl) ValidateWrite(
	key string, context ldcontext.Context, defaultStage MigrationStage,
) (interface{}, error, error) {
	stage, err := m.client.MigrationVariation(key, context, defaultStage)
	if err != nil {
		return nil, err, nil
	}

	// We do not want to run write operations in parallel
	runInParallel := false

	oldExecutor := &migrationExecutor{
		label:     "old",
		key:       key,
		impl:      m.writeConfig.old,
		latencyFn: m.oldLatencyFn,
	}
	newExecutor := &migrationExecutor{
		label:     "new",
		key:       key,
		impl:      m.writeConfig.new,
		latencyFn: m.newLatencyFn,
	}

	switch stage {
	case Off:
		result, err := oldExecutor.exec(context)
		return result, err, nil
	case DualWrite:
		return m.runBoth(key, context, *oldExecutor, *newExecutor, m.writeConfig.compare, runInParallel)
	case Shadow:
		return m.runBoth(key, context, *oldExecutor, *newExecutor, m.writeConfig.compare, runInParallel)
	case Live:
		return m.runBoth(key, context, *newExecutor, *oldExecutor, m.writeConfig.compare, runInParallel)
	case RampDown:
		return m.runBoth(key, context, *oldExecutor, *newExecutor, m.writeConfig.compare, runInParallel)
	case Complete:
		result, err := newExecutor.exec(context)
		return result, err, nil
	}

	return nil, nil, nil
}

func (m migratorImpl) runBoth(
	key string,
	context ldcontext.Context,
	active migrationExecutor,
	passive migrationExecutor,
	comparison MigrationComparisonFn,
	runInParallel bool,
) (interface{}, error, error) {
	var resultActive, resultPassive interface{}
	var errActive, errPassive error

	switch {
	case runInParallel:
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			resultActive, errActive = active.exec(context)
			defer wg.Done()
		}()

		go func() {
			resultPassive, errPassive = passive.exec(context)
			defer wg.Done()
		}()

		wg.Wait()
	//nolint:gosec // This doesn't have to cryptographically secure
	case m.randomizeSeqExecOrder && rand.Float32() > 0.5:
		resultPassive, errPassive = passive.exec(context)
		resultActive, errActive = active.exec(context)
	default:
		resultActive, errActive = active.exec(context)
		resultPassive, errPassive = passive.exec(context)
	}

	// QUESTION: Should we also be providing the errors here in case they want to compare those things?
	_ = m.client.TrackConsistency(key, context, comparison(resultActive, resultPassive))
	if errActive != nil || errPassive != nil {
		return nil, errActive, errPassive
	}
	return resultActive, nil, nil
}

type migrationExecutor struct {
	label     string
	key       string
	impl      MigrationImplFn
	latencyFn func(key string, context ldcontext.Context, elapsed time.Duration) error
}

func (e migrationExecutor) exec(context ldcontext.Context) (interface{}, error) {
	start := time.Now()
	result, err := e.impl()

	// QUESTION: How sure are we that we want to do this? If a call is failing
	// fast, the latency metric might look really good for the new version
	// quite some time after the fix was put in place.
	elapsed := time.Since(start)
	_ = e.latencyFn(e.key+"-latency-"+e.label, context, elapsed)

	if err != nil {
		return nil, err
	}
	return result, nil
}
