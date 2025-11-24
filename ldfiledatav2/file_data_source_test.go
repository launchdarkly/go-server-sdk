package ldfiledatav2

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/stretchr/testify/assert"
)

func TestSuccessfullyLoadsJsonFlags(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true}}, "segments": {"my-segment": {"rules": []}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		defer sync.Close()
		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		result := <-resultChan
		assert.NotNil(t, result.ChangeSet)
		assert.Len(t, result.ChangeSet.Changes(), 2)

		keys := map[string]bool{}
		for _, change := range result.ChangeSet.Changes() {
			keys[change.Key] = true
		}
		assert.Contains(t, keys, "my-flag")
		assert.Contains(t, keys, "my-segment")

		assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
		assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
	})
}

func TestSuccessfullyLoadsJsonFlagValues(t *testing.T) {
	th.WithTempFileData([]byte(`{"flagValues": {"my-flag": true}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		defer sync.Close()
		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		result := <-resultChan
		assert.NotNil(t, result.ChangeSet)
		assert.Len(t, result.ChangeSet.Changes(), 1)
		assert.Equal(t, "my-flag", result.ChangeSet.Changes()[0].Key)
		assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
		assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
	})
}

func TestSuccessfullyLoadsYamlFlags(t *testing.T) {
	fileData := `
---
flags:
  my-flag:
    "on": true
`
	th.WithTempFileData([]byte(fileData), func(filename string) {
		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		defer sync.Close()
		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		result := <-resultChan
		assert.NotNil(t, result.ChangeSet)
		assert.Len(t, result.ChangeSet.Changes(), 1)
		assert.Equal(t, "my-flag", result.ChangeSet.Changes()[0].Key)
		assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
		assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
	})
}

func TestSuccessfullyLoadsYamlFlagValues(t *testing.T) {
	fileData := `
---
flagValues:
  my-flag: true
`
	th.WithTempFileData([]byte(fileData), func(filename string) {
		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		defer sync.Close()
		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		result := <-resultChan
		assert.NotNil(t, result.ChangeSet)
		assert.Len(t, result.ChangeSet.Changes(), 1)
		assert.Equal(t, "my-flag", result.ChangeSet.Changes()[0].Key)
		assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
		assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
	})
}

func TestSuccessfullyLoadsJsonFlagsFromMultipleFiles(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true}}}`), func(filename1 string) {
		th.WithTempFileData([]byte(`{"flags": {"your-flag": {"on": true}}}`), func(filename2 string) {
			factory := DataSource().FilePaths(filename1, filename2)
			sync, err := factory.Build(subsystems.BasicClientContext{})
			assert.NoError(t, err)

			defer sync.Close()
			resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

			result := <-resultChan
			assert.NotNil(t, result.ChangeSet)
			assert.Len(t, result.ChangeSet.Changes(), 2)

			keys := map[string]bool{}
			for _, change := range result.ChangeSet.Changes() {
				keys[change.Key] = true
			}
			assert.Contains(t, keys, "my-flag")
			assert.Contains(t, keys, "your-flag")

			assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
			assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
			assert.Equal(t, interfaces.DataSourceStateValid, result.State)
			assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
		})
	})
}

func TestIsInterruptedWhenLoadingConflictingFiles(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true, "version": 1}}}`), func(filename1 string) {
		th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": false, "version": 2}, "your-flag": {"on": false}}}`), func(filename2 string) {
			factory := DataSource().FilePaths(filename1, filename2)
			sync, err := factory.Build(subsystems.BasicClientContext{})
			assert.NoError(t, err)

			defer sync.Close()
			resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

			result := <-resultChan
			assert.Nil(t, result.ChangeSet)

			assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)
			assert.Equal(t, interfaces.DataSourceErrorKindInvalidData, result.Error.Kind)
		})
	})
}

func TestIsValidWhenLoadingConflictingFilesWhenIgnoringDuplicates(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true, "version": 1}}}`), func(filename1 string) {
		th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": false, "version": 2}, "your-flag": {"on": false}}}`), func(filename2 string) {
			factory := DataSource().FilePaths(filename1, filename2).DuplicateKeysHandling(DuplicateKeysIgnoreAllButFirst)
			sync, err := factory.Build(subsystems.BasicClientContext{})
			assert.NoError(t, err)

			defer sync.Close()
			resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

			result := <-resultChan
			assert.NotNil(t, result.ChangeSet)
			assert.Len(t, result.ChangeSet.Changes(), 2)

			keys := map[string]subsystems.Change{}
			for _, change := range result.ChangeSet.Changes() {
				keys[change.Key] = change
			}
			assert.Contains(t, keys, "my-flag")
			assert.Contains(t, keys, "your-flag")
			// It uses the first duplicate key.
			assert.Equal(t, 1, keys["my-flag"].Version)

			assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
			assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
			assert.Equal(t, interfaces.DataSourceStateValid, result.State)
			assert.Equal(t, interfaces.DataSourceErrorInfo{}, result.Error)
		})
	})
}

func TestStatusIsInterruptedOnInvalidData(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags"`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		defer sync.Close()
		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		result := <-resultChan
		assert.Nil(t, result.ChangeSet)
		assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)
		assert.Equal(t, interfaces.DataSourceErrorKindUnknown, result.Error.Kind)
	})
}

func TestRecoversFromInvalidDataWhenFileUpdated(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags"`), func(invalidJson string) {
		th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": false}}}`), func(validJson string) {
			reloadCh := make(chan any)
			f := func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
				go func() {
					<-reloadCh
					reload()
				}()
				return nil
			}

			factory := DataSource().FilePaths(invalidJson).Reloader(f)
			sync, err := factory.Build(subsystems.BasicClientContext{})
			assert.NoError(t, err)

			defer sync.Close()
			resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

			result := <-resultChan
			assert.Nil(t, result.ChangeSet)
			assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)

			assert.NoError(t, os.Rename(validJson, invalidJson))

			reloadCh <- struct{}{}

			result = <-resultChan
			assert.NotNil(t, result.ChangeSet)
			assert.Len(t, result.ChangeSet.Changes(), 1)
			assert.Equal(t, "my-flag", result.ChangeSet.Changes()[0].Key)
			assert.Equal(t, subsystems.NoSelector(), result.ChangeSet.Selector())
			assert.Equal(t, subsystems.IntentTransferFull, result.ChangeSet.IntentCode())
			assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		})
	})
}

func TestStatusIsInterruptedOnMissingFile(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags"`), func(filename string) {
		assert.NoError(t, os.Remove(filename))

		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		defer sync.Close()
		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		result := <-resultChan
		assert.Nil(t, result.ChangeSet)
		assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)
		assert.Equal(t, interfaces.DataSourceErrorKindUnknown, result.Error.Kind)
	})
}

func TestClosingSynchronizerClosesResultsChannel(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true}}, "segments": {"my-segment": {"rules": []}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		sync, err := factory.Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

		<-resultChan
		sync.Close()

		th.AssertChannelClosed(t, resultChan, time.Millisecond, "result channel should be closed")
	})
}

func TestSuccessfullyLoadsJsonFlagsAsInitializer(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true}}, "segments": {"my-segment": {"rules": []}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		initializer, err := factory.AsInitializer().Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		basis, err := initializer.Fetch(mocks.NewMockDataSelector(subsystems.NoSelector()), context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, basis)

		assert.Len(t, basis.ChangeSet.Changes(), 2)

		keys := map[string]bool{}
		for _, change := range basis.ChangeSet.Changes() {
			keys[change.Key] = true
		}
		assert.Contains(t, keys, "my-flag")
		assert.Contains(t, keys, "my-segment")

		assert.Equal(t, subsystems.NoSelector(), basis.ChangeSet.Selector())
		assert.Equal(t, subsystems.IntentTransferFull, basis.ChangeSet.IntentCode())
	})
}

func TestInitializerReturnsErrorIfFileDoesNotExist(t *testing.T) {
	th.WithTempFileData([]byte(``), func(filename string) {
		assert.NoError(t, os.Remove(filename))

		factory := DataSource().FilePaths(filename)
		initializer, err := factory.AsInitializer().Build(subsystems.BasicClientContext{})
		assert.NoError(t, err)

		_, err = initializer.Fetch(mocks.NewMockDataSelector(subsystems.NoSelector()), context.Background())
		assert.Error(t, err)
	})
}
