package ldfiledata

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// ReloaderFactory is a function type used with DataSourceBuilder.Reloader, to specify a mechanism for
// detecting when data files should be reloaded. Its standard implementation is in the ldfilewatch package.
type ReloaderFactory func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error

// DataSourceBuilder is a builder for configuring the file-based data source.
//
// Obtain an instance of this type by calling DataSource(). After calling its methods to specify any
// desired custom settings, store it in the SDK configuration's DataSource field.
//
// Builder calls can be chained, for example:
//
//     config.DataStore = ldfiledata.DataSource().FilePaths("file1").FilePaths("file2")
//
// You do not need to call the builder's CreatePersistentDataSource() method yourself; that will be
// done by the SDK.
type DataSourceBuilder struct {
	filePaths       []string
	reloaderFactory ReloaderFactory
}

// DataSource returns a configurable builder for a file-based data source.
func DataSource() *DataSourceBuilder {
	return &DataSourceBuilder{}
}

// FilePaths specifies the input data files. The paths may be any number of absolute or relative file paths.
func (b *DataSourceBuilder) FilePaths(paths ...string) *DataSourceBuilder {
	b.filePaths = append(b.filePaths, paths...)
	return b
}

// Reloader specifies a mechanism for reloading data files.
//
// It is normally used with the ldfilewatch package, as follows:
//
//     config := ld.Config{
//         DataSource: ldfiledata.DataSource().
//             FilePaths(filePaths).
//             Reloader(ldfilewatch.WatchFiles),
//     }
func (b *DataSourceBuilder) Reloader(reloaderFactory ReloaderFactory) *DataSourceBuilder {
	b.reloaderFactory = reloaderFactory
	return b
}

// CreateDataSource is called by the SDK to create the data source instance.
func (b *DataSourceBuilder) CreateDataSource(
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	return newFileDataSourceImpl(context, dataSourceUpdates, b.filePaths, b.reloaderFactory)
}
