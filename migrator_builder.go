package ldclient

import (
	"errors"
)

// MigratorBuilder provides a mechanism to construct a Migrator instance.
type MigratorBuilder struct {
	client             *LDClient
	readExecutionOrder ExecutionOrder

	measureLatency bool
	measureErrors  bool

	readConfig  *migrationConfig
	writeConfig *migrationConfig
}

// Migration creates a new MigratorBuilder instance with sane defaults.
//
// The builder defaults to tracking latency and error metrics, and will execute
// multiple migration-reads concurrently when possible.
func Migration(client *LDClient) *MigratorBuilder {
	return &MigratorBuilder{
		client:             client,
		measureLatency:     true,
		measureErrors:      true,
		readExecutionOrder: Concurrently,
	}
}

// ReadExecutionOrder influences the level of concurrency when the migration stage calls for multiple execution reads.
func (b *MigratorBuilder) ReadExecutionOrder(order ExecutionOrder) *MigratorBuilder {
	b.readExecutionOrder = order
	return b
}

// TrackLatency can be used to enable or disable latency tracking methods. Tracking is enabled by default.
func (b *MigratorBuilder) TrackLatency(enabled bool) *MigratorBuilder {
	b.measureLatency = enabled
	return b
}

// TrackErrors can be used to enable or disable error tracking. Tracking is enabled by default.
func (b *MigratorBuilder) TrackErrors(enabled bool) *MigratorBuilder {
	b.measureErrors = enabled
	return b
}

// Read can be used to configure the migration-read behavior of the resulting Migrator instance.
//
// Users are required to provide two different read methods -- one to read from the old migration source, and one to
// read from the new source. Additionally, customers can opt-in to consistency tracking by providing a comparison
// function.
//
// Depending on the migration stage, one or both of these read methods may be called.
func (b *MigratorBuilder) Read(
	oldReadFn, newReadFn MigrationImplFn,
	comparisonFn *MigrationComparisonFn,
) *MigratorBuilder {
	b.readConfig = &migrationConfig{
		old:     oldReadFn,
		new:     newReadFn,
		compare: comparisonFn,
	}
	return b
}

// Write can be used to configure the migration-write behavior of the resulting Migrator instance.
//
// Users are required to provide two different write methods -- one to write to the old migration source, and one to
// write to the new source. Not every stage requires
//
// Depending on the migration stage, one or both of these write methods may be called.
func (b *MigratorBuilder) Write(oldWriteFn, newWriteFn MigrationImplFn) *MigratorBuilder {
	b.writeConfig = &migrationConfig{
		old: oldWriteFn,
		new: newWriteFn,
	}
	return b
}

// Build constructs a Migrator instance to support migration-based reads and writes. An error will be returned if the
// build process fails.
func (b *MigratorBuilder) Build() (Migrator, error) {
	if b == nil {
		return nil, errors.New("calling build on nil pointer")
	}

	if b.client == nil {
		return nil, errors.New("a valid client was not provided")
	}

	if b.readConfig == nil {
		return nil, errors.New("no read configuration has been provided")
	}

	if b.writeConfig == nil {
		return nil, errors.New("no write configuration has been provided")
	}

	migrator := migratorImpl{
		client:             b.client,
		readExecutionOrder: b.readExecutionOrder,
		readConfig:         *b.readConfig,
		writeConfig:        *b.writeConfig,
		measureLatency:     b.measureLatency,
		measureErrors:      b.measureErrors,
	}

	return migrator, nil
}
