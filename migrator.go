package ldclient

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

type migratorImpl struct {
	client             *LDClient
	readExecutionOrder ExecutionOrder

	readConfig  migrationConfig
	writeConfig migrationConfig

	measureLatency bool
	measureErrors  bool
}

func (m migratorImpl) ValidateRead(
	key string, context ldcontext.Context, defaultStage MigrationStage,
) MigrationReadResult {
	stage, err := m.client.MigrationVariation(key, context, defaultStage)
	if err != nil {
		m.client.loggers.Error(err)
	}

	oldExecutor := &migrationExecutor{
		key:       key,
		origin:    Old,
		impl:      m.readConfig.old,
		latencyFn: func(_ string, _ ldcontext.Context, _ time.Duration) error { return nil },
	}
	newExecutor := &migrationExecutor{
		key:       key,
		origin:    New,
		impl:      m.readConfig.new,
		latencyFn: func(_ string, _ ldcontext.Context, _ time.Duration) error { return nil },
	}

	var readResult MigrationReadResult

	switch stage {
	case Off:
		readResult.MigrationResult = oldExecutor.exec(context)
		return readResult
	case DualWrite:
		readResult.MigrationResult = oldExecutor.exec(context)
		return readResult
	case Shadow:
		authoritativeResult, _ := m.runBoth(context, *oldExecutor, *newExecutor, m.readConfig.compare, m.readExecutionOrder)
		readResult.MigrationResult = authoritativeResult
		return readResult
	case Live:
		authoritativeResult, _ := m.runBoth(context, *newExecutor, *oldExecutor, m.readConfig.compare, m.readExecutionOrder)
		readResult.MigrationResult = authoritativeResult
		return readResult
	case RampDown:
		readResult.MigrationResult = newExecutor.exec(context)
		return readResult
	case Complete:
		readResult.MigrationResult = newExecutor.exec(context)
		return readResult
	}

	// NOTE: This should be unattainable if the above switch is exhaustive as it should be.
	readResult.MigrationResult = MigrationResult{
		error: fmt.Errorf("invalid stage %s detected; cannot execute read", stage),
	}

	return readResult
}

func (m migratorImpl) ValidateWrite(
	key string, context ldcontext.Context, defaultStage MigrationStage,
) MigrationWriteResult {
	stage, err := m.client.MigrationVariation(key, context, defaultStage)
	if err != nil {
		m.client.loggers.Error(err)
	}

	oldExecutor := &migrationExecutor{
		key:       key,
		impl:      m.writeConfig.old,
		latencyFn: func(_ string, _ ldcontext.Context, _ time.Duration) error { return nil },
	}
	newExecutor := &migrationExecutor{
		key:       key,
		impl:      m.writeConfig.new,
		latencyFn: func(_ string, _ ldcontext.Context, _ time.Duration) error { return nil },
	}

	switch stage {
	case Off:
		result := oldExecutor.exec(context)
		return NewMigrationWriteResult(result, nil)
	case DualWrite:
		authoritativeResult, nonAuthoritativeResult := m.runBoth(context, *oldExecutor, *newExecutor, nil, Serial)
		return NewMigrationWriteResult(authoritativeResult, &nonAuthoritativeResult)
	case Shadow:
		authoritativeResult, nonAuthoritativeResult := m.runBoth(context, *oldExecutor, *newExecutor, nil, Serial)
		return NewMigrationWriteResult(authoritativeResult, &nonAuthoritativeResult)
	case Live:
		authoritativeResult, nonAuthoritativeResult := m.runBoth(context, *newExecutor, *oldExecutor, nil, Serial)
		return NewMigrationWriteResult(authoritativeResult, &nonAuthoritativeResult)
	case RampDown:
		authoritativeResult, nonAuthoritativeResult := m.runBoth(context, *oldExecutor, *newExecutor, nil, Serial)
		return NewMigrationWriteResult(authoritativeResult, &nonAuthoritativeResult)
	case Complete:
		authoritativeResult := newExecutor.exec(context)
		return NewMigrationWriteResult(authoritativeResult, nil)
	}

	// NOTE: This should be unattainable if the above switch is exhaustive as it should be.
	return MigrationWriteResult{
		authoritative: MigrationResult{
			error: fmt.Errorf("invalid stage %s detected; cannot execute read", stage),
		},
	}
}

func (m migratorImpl) runBoth(
	context ldcontext.Context,
	authoritative, nonAuthoritative migrationExecutor,
	comparison *MigrationComparisonFn,
	executionOrder ExecutionOrder,
) (MigrationResult, MigrationResult) {
	var authoritativeMigrationResult, nonAuthoritativeMigrationResult MigrationResult

	switch {
	case executionOrder == Concurrently:
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			authoritativeMigrationResult = authoritative.exec(context)
			defer wg.Done()
		}()

		go func() {
			nonAuthoritativeMigrationResult = nonAuthoritative.exec(context)
			defer wg.Done()
		}()

		wg.Wait()
	case executionOrder == Randomized && rand.Float32() > 0.5: //nolint:gosec // doesn't need cryptographic security
		nonAuthoritativeMigrationResult = nonAuthoritative.exec(context)
		authoritativeMigrationResult = authoritative.exec(context)
	default:
		authoritativeMigrationResult = authoritative.exec(context)
		nonAuthoritativeMigrationResult = nonAuthoritative.exec(context)
	}

	if comparison != nil {
		_ = (*comparison)(authoritativeMigrationResult.GetResult(), nonAuthoritativeMigrationResult.GetResult())
		// NOTE: Handle this consistency check
	}

	return authoritativeMigrationResult, nonAuthoritativeMigrationResult
}

type migrationExecutor struct {
	key       string
	origin    MigrationOrigin
	impl      MigrationImplFn
	latencyFn func(key string, context ldcontext.Context, elapsed time.Duration) error
}

func (e migrationExecutor) exec(context ldcontext.Context) MigrationResult {
	start := time.Now()
	result, err := e.impl()

	// QUESTION: How sure are we that we want to do this? If a call is failing
	// fast, the latency metric might look wonderful for the new version
	// quite some time after the fix was put in place.
	elapsed := time.Since(start)
	_ = e.latencyFn(e.key+"-latency-", context, elapsed)

	if err != nil {
		return NewErrorMigrationResult(e.origin, err)
	}

	return NewSuccessfulMigrationResult(e.origin, result)
}
