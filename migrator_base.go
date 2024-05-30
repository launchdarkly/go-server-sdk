package ldclient

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	ldevents "github.com/launchdarkly/go-server-sdk/ldevents/v4"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

// MigrationCapableClient represents the subset of operations required to perform a migration operation. This interface
// is satisfied by the LDClient.
type MigrationCapableClient interface {
	MigrationVariation(
		key string,
		context ldcontext.Context,
		defaultStage ldmigration.Stage,
	) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error)
	TrackMigrationOp(event ldevents.MigrationOpEventData) error
	Loggers() interfaces.LDLoggers
}

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
	origin  ldmigration.Origin
	result  interface{}
	error   error
}

// NewSuccessfulMigrationResult creates a migration result with a defined origin and an operation result value.
func NewSuccessfulMigrationResult(origin ldmigration.Origin, result interface{}) MigrationResult {
	return MigrationResult{
		origin:  origin,
		success: true,
		result:  result,
	}
}

// NewErrorMigrationResult creates a failed migration result with a defined origin and the error which caused the
// migration to fail.
func NewErrorMigrationResult(origin ldmigration.Origin, err error) MigrationResult {
	return MigrationResult{
		origin:  origin,
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
func (m MigrationResult) GetOrigin() ldmigration.Origin {
	return m.origin
}

// GetError returns the error responsible for causing the MigrationResult to fail.
func (m MigrationResult) GetError() error {
	return m.error
}

// MigrationComparisonFn is used to compare the results of two migration operations. If the provided results are equal,
// this method will return true and false otherwise.
type MigrationComparisonFn func(interface{}, interface{}) bool

// MigrationImplFn represents the customer defined migration operation function. This method is expected to return a
// meaningful value if the function succeeds, and an error otherwise.
type MigrationImplFn func(payload interface{}) (interface{}, error)

type migrationConfig struct {
	old     MigrationImplFn
	new     MigrationImplFn
	compare *MigrationComparisonFn
}

// Migrator represents the interface through which migration support is executed.
type Migrator interface {
	// Read uses the provided flag key and context to execute a migration-backed read operation.
	Read(
		key string, context ldcontext.Context, defaultStage ldmigration.Stage, payload interface{},
	) MigrationReadResult
	// Write uses the provided flag key and context to execute a migration-backed write operation.
	Write(
		key string, context ldcontext.Context, defaultStage ldmigration.Stage, payload interface{},
	) MigrationWriteResult
}
