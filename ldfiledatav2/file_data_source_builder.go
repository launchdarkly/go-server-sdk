package ldfiledatav2

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// ReloaderFactory is a function type used with DataSourceBuilder.Reloader, to specify a mechanism for
// detecting when data files should be reloaded. Its standard implementation is in the ldfilewatch package.
type ReloaderFactory func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error

// DuplicateKeysHandling is a parameter type used with DataSourceBuilder.DuplicateKeysHandling.
type DuplicateKeysHandling string

const (
	// DuplicateKeysFail is an option for DataSourceBuilder.DuplicateKeysHandling, meaning that data loading
	// should fail if keys are duplicated across files. This is the default behavior.
	DuplicateKeysFail DuplicateKeysHandling = "fail"

	// DuplicateKeysKeepFirst is an option for DataSourceBuilder.DuplicateKeysHandling, meaning that
	// if keys are duplicated across files the first occurrence will be used.
	DuplicateKeysKeepFirst DuplicateKeysHandling = "ignore"

	// DuplicateKeysIgnoreAllButFirst is an older name for [DuplicateKeysKeepFirst].
	//
	// Deprecated: Use [DuplicateKeysKeepFirst] instead.
	DuplicateKeysIgnoreAllButFirst DuplicateKeysHandling = "ignore"
)

// DataSourceBuilder is a builder for configuring the file-based data source.
//
// Obtain an instance of this type by calling [DataSource]. After calling its methods to specify any
// desired custom settings, store it in the DataSource field of [github.com/launchdarkly/go-server-sdk/v7.Config].
//
// Builder calls can be chained, for example:
//
//	config.DataStore = ldfiledata.DataSource().FilePaths("file1").FilePaths("file2")
//
// You do not need to call the builder's Build method yourself; that will be done by the SDK.
type DataSourceBuilder struct {
	filePaths             []string
	duplicateKeysHandling DuplicateKeysHandling
	reloaderFactory       ReloaderFactory
}

// DataSource returns a configurable builder for a file-based data source.
func DataSource() *DataSourceBuilder {
	return &DataSourceBuilder{duplicateKeysHandling: DuplicateKeysFail}
}

// DuplicateKeysHandling specifies how to handle keys that are duplicated across files.
//
// If this is not specified, or if you set it to an unrecognized value, the default is DuplicateKeysFail.
func (b *DataSourceBuilder) DuplicateKeysHandling(duplicateKeysHandling DuplicateKeysHandling) *DataSourceBuilder {
	b.duplicateKeysHandling = duplicateKeysHandling
	return b
}

// FilePaths specifies the input data files. The paths may be any number of absolute or relative file paths.
func (b *DataSourceBuilder) FilePaths(paths ...string) *DataSourceBuilder {
	b.filePaths = append(b.filePaths, paths...)
	return b
}

// Reloader specifies a mechanism for reloading data files.
//
// It is normally used with the [github.com/launchdarkly/go-server-sdk/v7/ldfilewatch] package, as follows:
//
//	config := ld.Config{
//	    DataSource: ldfiledata.DataSource().
//	        FilePaths(filePaths).
//	        Reloader(ldfilewatch.WatchFiles),
//	}
func (b *DataSourceBuilder) Reloader(reloaderFactory ReloaderFactory) *DataSourceBuilder {
	b.reloaderFactory = reloaderFactory
	return b
}

// Build creates a DataSynchronizer instance that can be used by the SDK client.
func (b *DataSourceBuilder) Build(clientContext subsystems.ClientContext) (subsystems.DataSynchronizer, error) {
	return newFileDataSourceImpl(clientContext, b.filePaths, b.duplicateKeysHandling, b.reloaderFactory)
}

// AsInitializer returns a ComponentConfigurer that can be used to register this DataSourceBuilder
// as a data initializer in the SDK's data system.
func (b *DataSourceBuilder) AsInitializer() subsystems.ComponentConfigurer[subsystems.DataInitializer] {
	return subsystems.AsInitializer(b)
}
