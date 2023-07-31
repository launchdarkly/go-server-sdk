package ldclient

import "github.com/launchdarkly/go-sdk-common/v3/ldcontext"

// ExecutionOrder represents the various execution modes this SDK can operate
// under while performing migration-assisted reads.
type ExecutionOrder uint8

const (
	// Serial execution ensures the authoritative read will always complete execution before executing the
	// non-authoritative read.
	Serial ExecutionOrder = iota
	// Randomized execution randomly decides if the authoritative read should execute first or second.
	Randomized
	// Concurrently executes both reads in separate go routines, and waits until both calls have finished before
	// proceeding.
	Concurrently
)

// MigrationOrigin represents the source of origin for a migration-related operation.
type MigrationOrigin int

const (
	// Old represents the technology source we are migrating away from.
	Old MigrationOrigin = iota
	// New represents the technology source we are migrating towards.
	New
)

// MigrationWriteResult contains the results of a migration write operation.
//
// Authoritative writes are done before non-authoritative, so the Authoritative field should contain either an error or
// a result.
//
// If the authoritative write fails, then the non-authoritative operation will not be executed. When this happens the
// NonAuthoritative field will not be populated.
//
// When the non-authoritative operation is executed, then it will result in either a result or an error and the field
// will be populated as such.
type MigrationWriteResult struct {
	authoritative    MigrationResult
	nonAuthoritative *MigrationResult
}

// NewMigrationWriteResult constructs a new write result containing the required authoritative result, and an optional
// non-authoritative result.
func NewMigrationWriteResult(authoritative MigrationResult, nonAuthoritative *MigrationResult) MigrationWriteResult {
	return MigrationWriteResult{authoritative, nonAuthoritative}
}

// GetAuthoritativeResult returns the result of an authoritative operation.
//
// Because authoritative operations are always run first, a MigrationWriteResult is guaranteed to have an authoritative
// result.
func (m MigrationWriteResult) GetAuthoritativeResult() MigrationResult {
	return m.authoritative
}

// GetNonAuthoritativeResult returns the result of a non-authoritative operation, if it was executed.
func (m MigrationWriteResult) GetNonAuthoritativeResult() *MigrationResult {
	return m.nonAuthoritative
}

// MigrationReadResult contains the results of a migration read operation.
//
// While an individual migration-backed read may execute multiple read operations, only the result related to the
// authoritative result is returned.
type MigrationReadResult struct {
	MigrationResult
}

// NewMigrationReadResult constructs a new read result containing the required authoritative result.
func NewMigrationReadResult(result MigrationResult) MigrationReadResult {
	return MigrationReadResult{result}
}

// MigrationResult represents the result of executing a migration-backed operation (either reads or writes).
type MigrationResult struct {
	success bool
	origin  MigrationOrigin
	result  interface{}
	error   error
}

// NewSuccessfulMigrationResult creates a migration result with a defined origin and an operation result value.
func NewSuccessfulMigrationResult(origin MigrationOrigin, result interface{}) MigrationResult {
	return MigrationResult{
		success: true,
		result:  result,
	}
}

// NewErrorMigrationResult creates a failed migration result with a defined origin and the error which caused the
// migration to fail.
func NewErrorMigrationResult(origin MigrationOrigin, err error) MigrationResult {
	return MigrationResult{
		success: false,
		error:   err,
	}
}

// IsSuccess returns true if the result was successful.
func (m MigrationResult) IsSuccess() bool {
	return m.success
}

// GetResult returns any result value associated with this MigrationResult.
func (m MigrationResult) GetResult() interface{} {
	return m.result
}

// GetOrigin returns the origin value associated with this result. Callers can use this to determine which technology
// source was modified during a migration operation.
func (m MigrationResult) GetOrigin() MigrationOrigin {
	return m.origin
}

// GetError returns the error responsible for causing the MigrationResult to fail.
func (m MigrationResult) GetError() error {
	return m.error
}

// MigrationStage denotes one of six possible stages a technology migration could be a part of.
type MigrationStage int

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

// String converts a MigrationStage into its string representation.
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

// MigrationComparisonFn is used to compare the results of two migration operations. If the provided results are equal,
// this method will return true and false otherwise.
type MigrationComparisonFn func(interface{}, interface{}) bool

// MigrationImplFn represents the customer defined migration operation function. This method is expected to return a
// meaningful value if the function succeeds, and an error otherwise.
type MigrationImplFn func() (interface{}, error)

type migrationConfig struct {
	old     MigrationImplFn
	new     MigrationImplFn
	compare *MigrationComparisonFn
}

// Migrator represents the interface through which migration support is executed.
type Migrator interface {
	// ValidateRead uses the provided flag key and context to execute a migration-backed read operation.
	ValidateRead(key string, context ldcontext.Context, defaultStage MigrationStage) MigrationReadResult
	// ValidateWrite uses the provided flag key and context to execute a migration-backed write operation.
	ValidateWrite(key string, context ldcontext.Context, defaultStage MigrationStage) MigrationWriteResult
}
