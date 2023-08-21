package ldclient

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

type migratorImpl struct {
	client             MigrationCapableClient
	readExecutionOrder ExecutionOrder

	readConfig  migrationConfig
	writeConfig migrationConfig

	measureLatency bool
	measureErrors  bool
}

func (m *migratorImpl) ValidateRead(
	key string, context ldcontext.Context, defaultStage ldmigration.Stage,
) MigrationReadResult {
	stage, tracker, err := m.client.MigrationVariation(key, context, defaultStage)
	tracker.Operation(ldmigration.Read)

	if err != nil {
		m.client.Loggers().Error(err)
	}

	oldExecutor := &migrationExecutor{
		key:            key,
		origin:         ldmigration.Old,
		impl:           m.readConfig.old,
		tracker:        tracker,
		measureLatency: m.measureLatency,
		measureErrors:  m.measureErrors,
	}
	newExecutor := &migrationExecutor{
		key:            key,
		origin:         ldmigration.New,
		impl:           m.readConfig.new,
		tracker:        tracker,
		measureLatency: m.measureLatency,
		measureErrors:  m.measureErrors,
	}

	var readResult MigrationReadResult

	switch stage {
	case ldmigration.Off:
		readResult.MigrationResult = oldExecutor.exec(context)
	case ldmigration.DualWrite:
		readResult.MigrationResult = oldExecutor.exec(context)
	case ldmigration.Shadow:
		authoritativeResult, _ := m.readFromBoth(context, *oldExecutor, *newExecutor, m.readConfig.compare, m.readExecutionOrder, tracker)
		readResult.MigrationResult = authoritativeResult
	case ldmigration.Live:
		authoritativeResult, _ := m.readFromBoth(context, *newExecutor, *oldExecutor, m.readConfig.compare, m.readExecutionOrder, tracker)
		readResult.MigrationResult = authoritativeResult
	case ldmigration.RampDown:
		readResult.MigrationResult = newExecutor.exec(context)
	case ldmigration.Complete:
		readResult.MigrationResult = newExecutor.exec(context)
	default:
		// NOTE: This should be unattainable if the above switch is exhaustive as it should be.
		readResult.MigrationResult = MigrationResult{
			error: fmt.Errorf("invalid stage %s detected; cannot execute read", stage),
		}
	}

	m.trackMigrationOp(tracker)

	return readResult
}

func (m *migratorImpl) ValidateWrite(
	key string, context ldcontext.Context, defaultStage ldmigration.Stage,
) MigrationWriteResult {
	stage, tracker, err := m.client.MigrationVariation(key, context, defaultStage)
	tracker.Operation(ldmigration.Write)
	if err != nil {
		m.client.Loggers().Error(err)
	}

	oldExecutor := &migrationExecutor{
		key:            key,
		impl:           m.writeConfig.old,
		tracker:        tracker,
		measureLatency: m.measureLatency,
		measureErrors:  m.measureErrors,
	}
	newExecutor := &migrationExecutor{
		key:            key,
		impl:           m.writeConfig.new,
		tracker:        tracker,
		measureLatency: m.measureLatency,
		measureErrors:  m.measureErrors,
	}

	var writeResult MigrationWriteResult

	switch stage {
	case ldmigration.Off:
		result := oldExecutor.exec(context)
		writeResult = NewMigrationWriteResult(result, nil)
	case ldmigration.DualWrite:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(context, *oldExecutor, *newExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.Shadow:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(context, *oldExecutor, *newExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.Live:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(context, *newExecutor, *oldExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.RampDown:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(context, *newExecutor, *oldExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.Complete:
		authoritativeResult := newExecutor.exec(context)
		writeResult = NewMigrationWriteResult(authoritativeResult, nil)
	default:
		// NOTE: This should be unattainable if the above switch is exhaustive as it should be.
		writeResult = MigrationWriteResult{
			authoritative: MigrationResult{
				error: fmt.Errorf("invalid stage %s detected; cannot execute read", stage),
			},
		}
	}

	m.trackMigrationOp(tracker)

	return writeResult
}

func (m *migratorImpl) trackMigrationOp(tracker interfaces.LDMigrationOpTracker) {
	event, err := tracker.Build()
	if err != nil {
		m.client.Loggers().Errorf("migration op failed to send; %v", err)
		return
	}

	if err := m.client.TrackMigrationOp(*event); err != nil {
		m.client.Loggers().Errorf("migration op failed to send; %v", err)
	}
}

func (m *migratorImpl) writeToBoth(
	context ldcontext.Context,
	authoritative, nonAuthoritative migrationExecutor,
) (MigrationResult, *MigrationResult) {
	var authoritativeMigrationResult, nonAuthoritativeMigrationResult MigrationResult

	authoritativeMigrationResult = authoritative.exec(context)
	if !authoritativeMigrationResult.IsSuccess() {
		return authoritativeMigrationResult, nil
	}

	nonAuthoritativeMigrationResult = nonAuthoritative.exec(context)
	return authoritativeMigrationResult, &nonAuthoritativeMigrationResult
}

func (m *migratorImpl) readFromBoth(
	context ldcontext.Context,
	authoritative, nonAuthoritative migrationExecutor,
	comparison *MigrationComparisonFn,
	executionOrder ExecutionOrder,
	tracker interfaces.LDMigrationOpTracker,
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
		wasConsistent := (*comparison)(authoritativeMigrationResult.GetResult(), nonAuthoritativeMigrationResult.GetResult())
		// TODO: need to figure out this samplingRatio stuff
		tracker.TrackConsistency(wasConsistent, 1)
	}

	return authoritativeMigrationResult, nonAuthoritativeMigrationResult
}

type migrationExecutor struct {
	key            string
	origin         ldmigration.Origin
	impl           MigrationImplFn
	tracker        interfaces.LDMigrationOpTracker
	measureLatency bool
	measureErrors  bool
}

func (e migrationExecutor) exec(context ldcontext.Context) MigrationResult {
	start := time.Now()
	result, err := e.impl()

	// QUESTION: How sure are we that we want to do this? If a call is failing
	// fast, the latency metric might look wonderful for the new version
	// quite some time after the fix was put in place.
	if e.measureLatency {
		e.tracker.TrackLatency(e.origin, time.Since(start))
	}

	if e.measureErrors {
		e.tracker.TrackError(e.origin, err != nil)
	}

	if err != nil {
		return NewErrorMigrationResult(e.origin, err)
	}

	return NewSuccessfulMigrationResult(e.origin, result)
}
