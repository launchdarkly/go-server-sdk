// Package ldtestdatav2 provides a mechanism for providing dynamically updatable feature flag state in a
// simplified form to an SDK client in test scenarios. The entry point for using this feature is
// [DataSource].
//
// Unlike the file data source (in the [github.com/launchdarkly/go-server-sdk/v7/ldfiledata] package),
// this mechanism does not use any external resources. It provides only the data that the application
// has put into it using the Update method.
//
//	td := ldtestdatav2.DataSource()
//	td.Update(td.Flag("flag-key-1").BooleanFlag().VariationForAll(true))
//
//	config := ld.Config{
//		DataSystem: ldcomponents.DataSystem().Custom().Synchronizers(td, nil)
//	}
//	client := ld.MakeCustomClient(sdkKey, config, timeout)
//
//	// flags can be updated at any time:
//	td.Update(td.Flag("flag-key-2").
//		VariationForUser("some-user-key", true).
//		FallthroughVariation(false))
//
// The above example uses a simple boolean flag, but more complex configurations are possible using
// the methods of the [FlagBuilder] that is returned by [TestDataSynchronizer.Flag]. FlagBuilder supports many of
// the ways a flag can be configured on the LaunchDarkly dashboard, but does not currently support 1.
// rule operators other than "in" and "not in", or 2. percentage rollouts.
//
// If the same TestDataSource instance is used to configure multiple LDClient instances, any change
// made to the data will propagate to all of the LDClients.
//
// WARNING: This particular implementation supports the upcoming flag delivery v2 format which is not
// publicly available.
//
// This package is not stable, and not subject to any backwards compatibility guarantees or semantic versioning.
// It is not suitable for production usage. Do not use it. You have been warned.
package ldtestdatav2
