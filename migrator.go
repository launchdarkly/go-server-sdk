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
	readExecutionOrder ldmigration.ExecutionOrder

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
		readResult.MigrationResult = oldExecutor.exec()
	case ldmigration.DualWrite:
		readResult.MigrationResult = oldExecutor.exec()
	case ldmigration.Shadow:
		authoritativeResult := m.readFromBoth(*oldExecutor, *newExecutor, m.readConfig.compare, m.readExecutionOrder, tracker)
		readResult.MigrationResult = authoritativeResult
	case ldmigration.Live:
		authoritativeResult := m.readFromBoth(*newExecutor, *oldExecutor, m.readConfig.compare, m.readExecutionOrder, tracker)
		readResult.MigrationResult = authoritativeResult
	case ldmigration.RampDown:
		readResult.MigrationResult = newExecutor.exec()
	case ldmigration.Complete:
		readResult.MigrationResult = newExecutor.exec()
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
		origin:         ldmigration.Old,
		impl:           m.writeConfig.old,
		tracker:        tracker,
		measureLatency: m.measureLatency,
		measureErrors:  m.measureErrors,
	}
	newExecutor := &migrationExecutor{
		key:            key,
		origin:         ldmigration.New,
		impl:           m.writeConfig.new,
		tracker:        tracker,
		measureLatency: m.measureLatency,
		measureErrors:  m.measureErrors,
	}

	var writeResult MigrationWriteResult

	switch stage {
	case ldmigration.Off:
		result := oldExecutor.exec()
		writeResult = NewMigrationWriteResult(result, nil)
	case ldmigration.DualWrite:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(*oldExecutor, *newExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.Shadow:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(*oldExecutor, *newExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.Live:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(*newExecutor, *oldExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.RampDown:
		authoritativeResult, nonAuthoritativeResult := m.writeToBoth(*newExecutor, *oldExecutor)
		writeResult = NewMigrationWriteResult(authoritativeResult, nonAuthoritativeResult)
	case ldmigration.Complete:
		authoritativeResult := newExecutor.exec()
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
	authoritative, nonAuthoritative migrationExecutor,
) (MigrationResult, *MigrationResult) {
	var authoritativeMigrationResult, nonAuthoritativeMigrationResult MigrationResult

	authoritativeMigrationResult = authoritative.exec()
	if !authoritativeMigrationResult.IsSuccess() {
		return authoritativeMigrationResult, nil
	}

	nonAuthoritativeMigrationResult = nonAuthoritative.exec()
	return authoritativeMigrationResult, &nonAuthoritativeMigrationResult
}

func (m *migratorImpl) readFromBoth(
	authoritative, nonAuthoritative migrationExecutor,
	comparison *MigrationComparisonFn,
	executionOrder ldmigration.ExecutionOrder,
	tracker interfaces.LDMigrationOpTracker,
) MigrationResult {
	var authoritativeMigrationResult, nonAuthoritativeMigrationResult MigrationResult

	switch {
	case executionOrder == ldmigration.Concurrent:
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			authoritativeMigrationResult = authoritative.exec()
			defer wg.Done()
		}()

		go func() {
			nonAuthoritativeMigrationResult = nonAuthoritative.exec()
			defer wg.Done()
		}()

		wg.Wait()
	case executionOrder == ldmigration.Random && rand.Float32() > 0.5: //nolint:gosec,lll // doesn't need cryptographic security
		nonAuthoritativeMigrationResult = nonAuthoritative.exec()
		authoritativeMigrationResult = authoritative.exec()
	default:
		authoritativeMigrationResult = authoritative.exec()
		nonAuthoritativeMigrationResult = nonAuthoritative.exec()
	}

	if comparison != nil {
		wasConsistent := (*comparison)(authoritativeMigrationResult.GetResult(), nonAuthoritativeMigrationResult.GetResult())
		tracker.TrackConsistency(wasConsistent)
	}

	return authoritativeMigrationResult
}

type migrationExecutor struct {
	key            string
	origin         ldmigration.Origin
	impl           MigrationImplFn
	tracker        interfaces.LDMigrationOpTracker
	measureLatency bool
	measureErrors  bool
}

func (e migrationExecutor) exec() MigrationResult {
	start := time.Now()
	result, err := e.impl()

	// QUESTION: How sure are we that we want to do this? If a call is failing
	// fast, the latency metric might look wonderful for the new version
	// quite some time after the fix was put in place.
	if e.measureLatency {
		e.tracker.TrackLatency(e.origin, time.Since(start))
	}

	if e.measureErrors && err != nil {
		e.tracker.TrackError(e.origin)
	}

	if err != nil {
		return NewErrorMigrationResult(e.origin, err)
	}

	return NewSuccessfulMigrationResult(e.origin, result)
}
