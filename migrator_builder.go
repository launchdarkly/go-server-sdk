package ldclient

import (
	"errors"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

// MigratorBuilder TKTK
type MigratorBuilder struct {
	client                *LDClient
	randomizeSeqExecOrder bool
	measureLatency        bool

	readConfig  *migrationConfig
	writeConfig *migrationConfig
}

// Migration TKTK
func Migration(client *LDClient) *MigratorBuilder {
	return &MigratorBuilder{
		client: client,
	}
}

// RandomizeExecution TKTK
func (b *MigratorBuilder) RandomizeExecution() *MigratorBuilder {
	b.randomizeSeqExecOrder = true
	return b
}

// TrackLatency TKTK
func (b *MigratorBuilder) TrackLatency() *MigratorBuilder {
	b.measureLatency = true
	return b
}

// Read TKTK
func (b *MigratorBuilder) Read(oldReadFn, newReadFn MigrationImplFn, comparisonFn MigrationComparisonFn) *MigratorBuilder {
	b.readConfig = &migrationConfig{
		old:     oldReadFn,
		new:     newReadFn,
		compare: comparisonFn,
	}
	return b
}

// Write TKTK
func (b *MigratorBuilder) Write(oldWriteFn, newWriteFn MigrationImplFn, comparisonFn MigrationComparisonFn) *MigratorBuilder {
	b.writeConfig = &migrationConfig{
		old:     oldWriteFn,
		new:     newWriteFn,
		compare: comparisonFn,
	}
	return b
}

// Build TKTK
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
		client:                b.client,
		randomizeSeqExecOrder: b.randomizeSeqExecOrder,
		readConfig:            *b.readConfig,
		writeConfig:           *b.writeConfig,
		oldLatencyFn:          func(_ string, _ ldcontext.Context, _ time.Duration) error { return nil },
		newLatencyFn:          func(_ string, _ ldcontext.Context, _ time.Duration) error { return nil },
	}

	if b.measureLatency {
		migrator.oldLatencyFn = b.client.TrackLatencyOldData
		migrator.newLatencyFn = b.client.TrackLatencyNewData
	}

	return migrator, nil
}
