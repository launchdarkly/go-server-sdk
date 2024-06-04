# Change log

All notable changes to the LaunchDarkly Go SDK will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [7.4.1](https://github.com/launchdarkly/go-server-sdk/compare/v7.4.0...v7.4.1) (2024-06-04)


### Bug Fixes

* add warning log message if client init duration is longer than recommended (60s) ([68e3440](https://github.com/launchdarkly/go-server-sdk/commit/68e3440f5368aa23e72345abc56cb77d7fe90914))
* align high timeout log msg with spec ([#150](https://github.com/launchdarkly/go-server-sdk/issues/150)) ([606d224](https://github.com/launchdarkly/go-server-sdk/commit/606d224c155833de2f2b38d5bea60de92e1765ea))
* resolve data race in Broadcasters system ([#153](https://github.com/launchdarkly/go-server-sdk/issues/153)) ([68cb1a4](https://github.com/launchdarkly/go-server-sdk/commit/68cb1a48724a9b739eca801e5ff9b64a8c6f6b14))

## [7.4.0](https://github.com/launchdarkly/go-server-sdk/compare/v7.3.0...v7.4.0) (2024-04-10)

This release contains no functional changes. It is a product of structural changes to the repository.

### Features

* Implement otel tracing hook. ([#130](https://github.com/launchdarkly/go-server-sdk/issues/130)) ([f5675c1](https://github.com/launchdarkly/go-server-sdk/commit/f5675c1d20976dc1f9dbb4064ab648abfd7765c2))

## [7.3.0](https://github.com/launchdarkly/go-server-sdk/compare/v7.2.0...v7.3.0) (2024-04-03)

This release introduces a Hooks API. Hooks are collections of user-defined callbacks that are executed by the SDK at various points of interest. You can use them to augment the SDK with metrics or tracing.

### Features

* Add support for hooks. ([#126](https://github.com/launchdarkly/go-server-sdk/issues/126)) ([929ef8b](https://github.com/launchdarkly/go-server-sdk/commit/929ef8b146dce99030a81033646f380a6b21fd98))
* Implement supporting types for hooks. ([#125](https://github.com/launchdarkly/go-server-sdk/issues/125)) ([7f06147](https://github.com/launchdarkly/go-server-sdk/commit/7f061475333cb09e9d59defc4046840af999b472))

## [7.2.0](https://github.com/launchdarkly/go-server-sdk/compare/v7.1.1...v7.2.0) (2024-03-25)


### Features

* **deps:** bump supported Go versions to 1.22 and 1.21 ([#114](https://github.com/launchdarkly/go-server-sdk/issues/114)) ([0ac6d4d](https://github.com/launchdarkly/go-server-sdk/commit/0ac6d4d006cdf1acb31ad727295bb825c204f464))

## [7.1.1](https://github.com/launchdarkly/go-server-sdk/compare/v7.1.0...v7.1.1) (2024-03-14)


### Bug Fixes

* do not emit error when RelayProxyEndpointsWithoutEvents is used ([#110](https://github.com/launchdarkly/go-server-sdk/issues/110)) ([0551bf1](https://github.com/launchdarkly/go-server-sdk/commit/0551bf115902f1f2d0691a060c1a16fc60ea7e69))

## [7.1.0](https://github.com/launchdarkly/go-server-sdk/compare/v7.0.1...v7.1.0) (2024-03-14)


### Features

* Inline contexts for all evaluation events ([#111](https://github.com/launchdarkly/go-server-sdk/issues/111)) ([0fad2bb](https://github.com/launchdarkly/go-server-sdk/commit/0fad2bb0790f711f3fd5014204490f5f36497dd2))
* Redact anonymous attributes within feature events ([0fad2bb](https://github.com/launchdarkly/go-server-sdk/commit/0fad2bb0790f711f3fd5014204490f5f36497dd2))

## [7.0.1](https://github.com/launchdarkly/go-server-sdk/compare/v7.0.0...v7.0.1) (2024-03-01)


### Bug Fixes

* **deps:** bump golang.org/x/crypto from 0.1.0 to 0.17.0 ([5d26f59](https://github.com/launchdarkly/go-server-sdk/commit/5d26f59f915c62636d7ba8b0a06e00a6c6c3f677))

## [7.0.0] - 2023-10-11
The latest version of this SDK supports the ability to manage migrations or modernizations, using migration flags. You might use this functionality if you are optimizing queries, upgrading to new tech stacks, migrating from one database to another, or other similar technology changes. Migration flags are part of LaunchDarkly's Early Access Program. This feature is available to all LaunchDarkly customers but may undergo additional changes before it is finalized.

### Added:
- New `Migrator` type which provides an out-of-the-box configurable migration framework.
- For more advanced use cases, added new `MigrationVariation` and `TrackMigrationOp` methods on `LDClient`.

### Changed:
- `LDClientEvaluations` interface now has a `MigrationVariation` method.
- `LDClientEvents` interface now has `TrackMigrationOp` method.

## [6.1.1] - 2023-10-11
### Fixed:
- Event payloads receiving an HTTP 413 status code will no longer prevent subsequent event payloads from being attempted.
- Readme Get started hyperlink destination. (Thanks, [christgf](https://github.com/launchdarkly/go-server-sdk/pull/96)!)

## [6.1.0] - 2023-04-13
### Added:
- Support for Payload Filtering in streaming and polling modes. Payload Filtering is a beta feature that allows SDKs to download a subset of environment data, rather than full environments.

## [6.0.3] - 2023-03-01
### Changed: 
- Bumped go-sdk-events to v2.0.1
- Bumped go-server-sdk-evaluation to v2.0.2

### Fixed:
- Bumped go-sdk-common to v3.0.1. This fixes a bug within the easyJSON unmarshaller when an unexpected privateAttributes or redactedAttributes was encountered in a Context _meta attribute.

## [6.0.2] - 2023-02-27
### Changed:
- Bumped golang/x/crypto and golang/x/sys to v0.1.0

## [6.0.1] - 2023-01-17
### Fixed:
- If the application configured custom base URIs that did not have a valid URI syntax, the SDK could panic at startup time. It will now simply log an error and be unable to connect to LaunchDarkly.
- Fixed shared test logic that is used by the database integration packages.

## [5.10.1] - 2023-01-17
### Fixed:
- If the application configured custom base URIs that did not have a valid URI syntax, the SDK could panic at startup time. It will now simply log an error and be unable to connect to LaunchDarkly.
- Fixed shared test logic that is used by the database integration packages.

## [6.0.0] - 2022-12-07
The latest version of this SDK supports LaunchDarkly's new custom contexts feature. Contexts are an evolution of a previously-existing concept, "users." Contexts let you create targeting rules for feature flags based on a variety of different information, including attributes pertaining to users, organizations, devices, and more. You can even combine contexts to create "multi-contexts." 

For detailed information about this version, please refer to the list below. For information on how to upgrade from the previous version, please read the [migration guide](https://docs.launchdarkly.com/sdk/server-side/go/migration-5-to-6).

### Added:
- In `go-sdk-common`, the new package `ldcontext` with the types `Context` and `Kind` defines the new context model.
- The `ldtestdata` flag builder methods have been extended to support now context-related options, such as matching a key for a specific context type other than "user".
- `LDClient.FlushAndWait()` is a synchronous version of `Flush()`.

### Changed _(breaking changes from 5.x)_:
- The SDK packages now use regular Go module import paths rather than `gopkg.in` paths: `gopkg.in/launchdarkly/go-server-sdk.v5` is replaced by `github.com/launchdarkly/go-server-sdk/v6` and `gopkg.in/launchdarkly/go-sdk-common.v2` is replaced by `github.com/launchdarkly/go-sdk-common/v3`.
- The type `lduser.User` has been redefined to be an alias for `ldcontext.Context`. This means that existing application code referencing `lduser.User` can still work as long as it is treating the user as an opaque value, and not calling methods on it that were specific to that type.
- All methods that used the type `lduser.User` now use `ldcontext.Context` instead.
- `lduser.NewUser` and `lduser.UserBuilder` now create an instance of `Context` instead of `User`. This is as a convenience so that any code that was previously using these methods to construct a user, but did _not_ reference the `User` type directly for the result, may still be usable without changes. It is still preferable to use the new constructors and builders for `Context`.
- The `Secondary` attribute which existed in `User` does not exist in `Context` and is no longer a supported feature.
- It was previously allowable to set a user key to an empty string. In the new context model, the key is not allowed to be empty. Trying to use an empty key will cause evaluations to fail and return the default value.
- If you were using JSON serialization to produce a representation of a `User`, the new type `Context` uses a different JSON schema, so any code that reads the JSON will need to be adjusted. If you are passing the JSON to other code that uses LaunchDarkly SDKs, make sure you have updated all SDKs to versions that use the new context model. (However, _unmarshaling_ a `Context` from JSON data will still work correctly even if the JSON is in the old user format.)
- Types such as `DataStore`, which define the low-level interfaces of LaunchDarkly SDK components and allow implementation of custom components, have been moved out of the `interfaces` subpackage into a new `subsystems` subpackage. Some types have been removed by using generics: for instance, the interface type `DataSourceFactory` has been replaced by `ComponentConfigurer[DataSource]`. Application code normally does not refer to these types except possibly to hold a value for a configuration property such as `Config.DataStore`, so this change is likely to only affect configuration-related logic.

### Changed (requirements/dependencies/build):
- The minimum Go version is now 1.18.

### Changed (behavioral changes):
- The SDK can now evaluate segments that have rules referencing other segments.
- Analytics event data now uses a new JSON schema due to differences between the context model and the old user model.

### Removed:
- Removed all types, fields, and methods that were deprecated as of the most recent 5.x release.
- Removed the `Secondary` meta-attribute in `lduser.User` and `lduser.UserBuilder`.
- The `Alias` method no longer exists because alias events are not needed in the new context model.
- The `InlineUsersInEvents` option no longer exists because it is not relevant in the new context model.

## [5.10.0] - 2022-07-05
### Added:
- `ldtestdata.FlagBuilder.VariationForAll` and `VariationForAllIndex`: new names for the deprecated methods listed below.

### Changed:
- It was possible to cause analytics event data to be lost by passing `ldvalue.Raw(nil)` or `ldvalue.Raw(json.RawMessage(""))` as a default value parameter to `JSONVariation`, or as the value of a custom user attribute, because a zero-length byte array is not a valid JSON value and would cause the event output to be malformed. The SDK now guards against this by encoding such a value as `null` in the event data. It is still possible to cause a similar problem by passing a malformed raw value as as `ldvalue.Raw(json.RawMessage("{{{"))`; it is the caller's responsibility not to do so, as the purpose of `json.RawMessage` is to avoid unnecessary parsing and implies that the value is known to be syntactically valid.

### Fixed:
- If a field in `Config.ApplicationInfo` is set to a string longer than 64 characters, the SDK will now log a warning and discard it, since the LaunchDarkly services cannot process such strings for these fields.
- Prevented a potential data race in `LDClient.Initialized()`. ([#69](https://github.com/launchdarkly/go-server-sdk/issues/69))

### Deprecated:
- `ldtestdata.FlagBuilder.VariationForAllUsers` and `VariationForAllUsersIndex`. These methods are being renamed because in the future, there will be other possible kinds of evaluation inputs that are not users, and these test methods will apply equally to those.

## [5.9.0] - 2022-03-22
### Added:
- `Config.ApplicationInfo`, for configuration of application metadata that may be used in LaunchDarkly analytics or other product features. This does not affect feature flag evaluations.

## [5.8.1] - 2022-01-19
### Fixed:
- When using `AllFlagsState` to produce bootstrap data for the JavaScript SDK, the Go SDK was not returning the correct metadata for evaluations that involved an experiment. As a result, the analytics events produced by the JavaScript SDK did not correctly reflect experimentation results.

## [5.8.0] - 2022-01-10
### Added:
- `Config.ServiceEndpoints` provides a simpler way of setting custom service base URIs, if you are connecting to a LaunchDarkly Relay Proxy instance, a private LaunchDarkly instance, or a test fixture. Previously, this required setting a `BaseURI` property for each individual service (streaming, events, etc.). If using the Relay Proxy, simply remove any `BaseURI` calls in your SDK configuration and set the `ServiceEndpoints` property to `ldcomponents.RelayProxyEndpoints(myRelayProxyUri)` (use `RelayProxyEndpointsWithoutEvents` instead if your Relay Proxy instance does not have event forwarding enabled).

## [5.7.0] - 2022-01-07
### Added:
- `HTTPConfigurationBuilder.Header` allows adding custom HTTP headers to all of the SDK's requests.

### Changed:
- The SDK will now log a message at `Error` level whenever it detects flag data that should not be possible according to LaunchDarkly's validation rules (for instance, a rule that references a nonexistent variation index, or a circular reference in flag prerequisites). Previously, such messages were only logged at `Warn` level and only if `LoggingConfigurationBuilder.LogEvaluationErrors` was enabled; that is still the default behavior for other kinds of errors (such as "flag not found"), but malformed data is considered a more serious error because it should not be possible and may require assistance from LaunchDarkly support.

### Fixed:
- If the SDK detects a circular reference in flag prerequisites (for example, flag A has flag B as a prerequisite and vice versa), it will now refuse to evaluate the affected flags, returning the default value and an error reason of `MALFORMED_FLAG`. Previously, such a condition would cause infinite recursion and a stack overflow. LaunchDarkly does not allow circular references to be created in flag configurations, but under rare circumstances an SDK could incorrectly perceive such a state if it received updates out of order; or, the SDK could be configured to use data from a file that contained such references.

## [5.6.0] - 2021-08-20
### Added:
- Added support for better handling of updates in the Relay Proxy when using big segments. The only API changes for this are in the `ldstoreimpl` package, which is not meant for regular application use but is used by the Relay Proxy.

### Fixed:
- When using big segments, if a big segment store query for a user returned `nil`, the evaluator was treating that as an automatic exclusion for the user and skipping any rules that might exist in the segment. It should instead treat `nil` the same as an empty result.

## [5.5.0] - 2021-07-20
### Added:
- The SDK now supports evaluation of Big Segments. An Early Access Program for creating and syncing Big Segments from customer data platforms is available to enterprise customers.

## [5.4.0] - 2021-06-17
### Added:
- The SDK now supports the ability to control the proportion of traffic allocation to an experiment. This works in conjunction with a new platform feature now available to early access customers.

## [5.3.1] - 2021-06-03
### Fixed:
- Fixed a bug in JSON parsing that could cause floating-point numbers (in flag variation values, rule values, or user attributes if a user is being parsed from JSON) to be read incorrectly if the number format included an exponent and did not include a decimal point (for instance, `1e5`). Since there are several equally valid number formats in JSON (so `1e5` is exactly equivalent to `100000`), whether this bug showed up would depend on the format chosen by whatever software had most recently converted the number to JSON before it was re-read, which is hard to predict, but it would only be likely to happen with either integers that had more than four trailing zeroes or floating-point numbers with leading zeroes.

## [5.3.0] - 2021-04-22
### Added:
- When using the file data source, `ldfiledata.DataSourceBuilder.DuplicateKeysHandling` allows you to specify that duplicate flag keys should _not_ cause an error as they normally would.

## [5.2.1] - 2021-02-11
### Fixed:
- When deserializing feature flags from JSON, an explicit null value for the `rollout` property (as opposed to just omitting the property) was being treated as an error. The LaunchDarkly service endpoints do not ever send `rollout: null`, but it should be considered valid if encountered in JSON from some other source.

## [5.2.0] - 2021-01-26
### Added:
- Added the `Alias` method to `LDClient`. This can be used to associate two user objects for analytics purposes with an alias event.

## [5.1.4] - 2021-01-21
### Fixed:
- Starting in version 5.1.0, an Info-level log message `got put: {DATA}` was being logged upon making a stream connection, where `{DATA}` was the JSON representation of all of the feature flag data received from LaunchDarkly. This was unintentional and has been removed.

## [5.1.3] - 2021-01-20
### Fixed:
- When using semantic version operators, semantic version strings were being rejected by the SDK if they contained a zero digit in any position _after_ the first character of a numeric version component. For instance, `0.1.2` and `1.2.3` were accepted, and `01.2.3` was correctly rejected (leading zeroes for nonzero values are not allowed), but `10.2.3` was incorrectly rejected. This would cause the flag/segment clause to incorrectly return a &#34;match&#34; or &#34;no match&#34; result depending on how the clause was written.

## [5.1.2] - 2021-01-11
### Fixed:
- If you provide an SDK key that contains an invalid character such as a newline, `MakeClient` or `MakeCustomClient` will fail immediately with the error message `SDK key contains invalid characters`. Previously, it would try to use the invalid key in an HTTP request to LaunchDarkly, and the resulting error message (produced by the Go standard library) would be `net/http: invalid header field value [xxx] for key Authorization`, where `[xxx]` was the key you had provided; that could be undesirable if for instance it was a real key that you had accidentally added a newline to, causing the actual SDK key to be visible in the error message in your application log.
- The `ldfilewatch` package, for using the file data source in auto-update mode, produced some unnecessary and cryptic log messages such as `waitForEvents` and `got close`. These have been removed. Instead, the file data source now logs the message `Reloading flag data after detecting a change` (at Info level) if a file has been updated. ([#48](https://github.com/launchdarkly/go-server-sdk/issues/48))

## [5.1.1] - 2021-01-04
### Fixed:
- Parsing a `User` from JSON failed if there was a `privateAttributeNames` property whose value was `null`. This has been fixed so that it behaves the same as if the property had a value of `[]` or if it was not present at all.

## [5.1.0] - 2020-12-17
### Added:
- The SDK now has an optional integration with the [EasyJSON](https://github.com/mailru/easyjson) library to increase the efficiency of JSON encoding and decoding. This is enabled if you add the build tag `launchdarkly_easyjson` to your build (`go build -tags launchdarkly_easyjson`); no other changes are required. If you do not set the build tag, the SDK does not reference any EasyJSON code.

### Changed:
- The SDK&#39;s default JSON encoding and decoding logic has been changed to use the new LaunchDarkly library [go-jsonstream](https://github.com/launchdarkly/go-jsonstream) instead of Go&#39;s built-in `encoding/json`. This should reduce spikes in CPU and memory usage that could occur when the SDK is receiving JSON data from LaunchDarkly or from a database, especially in environments with many feature flags or complex flag configurations. There is also the option to use EasyJSON as described above, but even without that, the new implementation is much more efficient than `encoding/json`; depending on the size of the data, it can reduce execution time and heap allocations during JSON parsing by roughly 50%.

### Fixed:
- Removed an extra newline from the &#34;Reconnecting in...&#34; message that is logged for stream reconnections.

## [5.0.2] - 2020-10-20
### Fixed:
- When serializing JSON representations for _deleted_ flags and segments, properties with default values (such as false booleans or empty arrays) were being dropped entirely to save bandwidth. However, these representations may be consumed by SDKs other than the Go SDK, and some of the LaunchDarkly SDKs do not tolerate missing properties (PHP in particular), so this has been fixed to remain consistent with the less efficient behavior of Go SDK 4.x. This is very similar to a bug that was fixed in the 5.0.1 release, but it only affects deleted items; we store a placeholder for these (a tombstone) and the JSON for this was being created in a different code path from the one that was fixed.
- The types in `go-sdk-common.v2/lduser` and `go-sdk-common.v2/ldvalue` now provide more specific validation errors for JSON unmarshalling, as described in the [`go-sdk-common` 2.0.1 release notes](https://github.com/launchdarkly/go-sdk-common/releases/tag/2.0.1).

## [5.0.1] - 2020-10-08
### Fixed:
- When serializing flags and segments to JSON, properties with default values (such as false booleans or empty arrays) were being dropped entirely to save bandwidth. However, these representations may be consumed by SDKs other than the Go SDK, and some of the LaunchDarkly SDKs do not tolerate missing properties, so this has been fixed to remain consistent with the less efficient behavior of Go SDK 4.x.

## [5.0.0] - 2020-09-18
This is a major rewrite that introduces a cleaner API design, adds new features, and makes the SDK code easier to maintain and extend. See the [Go 4.x to 5.0 migration guide](https://docs.launchdarkly.com/sdk/server-side/go/migration-4-to-5) for an in-depth look at the changes in this version; the following is a summary.

### Added:
- You can tell the SDK to notify you whenever a feature flag&#39;s configuration has changed (either in general, or in terms of its result for a specific user), using [`LDClient.GetFlagTracker()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5#LDClient.GetFlagTracker).
- You can monitor the status of the SDK&#39;s data source (which normally means the streaming connection to the LaunchDarkly service) with [`LDClient.GetDataSourceStatusProvider()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5?tab=doc#LDClient.GetDataSourceStatusProvider). This allows you to check the current connection status, and to be notified if this status changes.
- You can monitor the status of a persistent data store with [`LDClient.GetDataStoreStatusProvider()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5?tab=doc#LDClient.GetDataStoreStatusProvider). This allows you to check whether database updates are succeeding, to be notified if this status changes, and to get caching statistics.
- [`LDClient.WithEventsDisabled()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5?tab=doc#LDClient.WithEventsDisabled) is a decorator that allows you to temporarily turn off analytics events even if events are enabled in your configuration.
- `LDConfig.Logging` is a new configuration category for options related to logging. See [`ldcomponents.Logging()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents#Logging) and [`ldcomponents.NoLogging()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents#NoLogging).
- The [`testhelpers/ldtestdata`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers/ldtestdata) package provides an alternative to `ldfiledata` for simulating feature flag data in test code.
- The object returned by `AllFlagsState()` can now be constructed (such as for testing purposes) with `flagstate.NewAllFlagsBuilder()`.
- In [`interfaces`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/interfaces), `LDClientInterface`, `LDClientEvaluations`, and `LDClientEvents` are new interfaces describing the existing methods of `LDClient`. These may be useful for creating mocks in testing.

### Changed _(breaking changes from 4.x)_:
- The `User` and `UserBuilder` types are now in the package [`gopkg.in/launchdarkly/go-sdk-common.v2/lduser`](https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/lduser). Users can no longer be created as inline structs; you must use `lduser.NewUser`, `lduser.NewAnonymousUser`, or `lduser.NewUserBuilder`.
- The `EvaluationDetail` and `EvaluationReason` types are now in the package [`gopkg.in/launchdarkly/go-sdk-common.v2/ldreason`](https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/ldreason).
- [`EvaluationDetail.VariationIndex`](https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/ldreason#EvaluationDetail) now uses the type [`ldvalue.OptionalInt`](https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue#OptionalInt) instead of the `*int` pointer type.
- [`EvaluationReason`](https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/ldreason#EvaluationReason) is now a struct rather than an interface, and is just one type rather than having separate types for each `Kind`.
- The `ldlog.Loggers` abstraction is now in the package [`gopkg.in/launchdarkly/go-sdk-common.v2/ldlog`](https://pkg.go.dev/gopkg.in/launchdarkly/go-sdk-common.v2/ldlog).
- Configuration properties that are specific to one mode of operation are no longer represented as fields in `Config`, but as builder methods on a component that is provided by the new `ldcomponents` package and is then placed in `Config`. For instance, instead of setting `Capacity: 1000`, you would now set `Events: ldcomponents.SendEvents().Capacity(1000)`; to disable events, instead of setting `SendEvents: false`, you would set `Events: ldcomponents.NoEvents()` (note that `NoEvents` does not allow you to set `Capacity`, since that would be meaningless if events are disabled). Similarly, to disable streaming and use polling, set `DataSource: ldcomponents.PollingDataSource()` which then provides optional methods for configuring polling. See `Config` and `ldcomponents` for more details.
- The database integrations that were formerly in the `redis`, `lddynamodb`, and `ldconsul` subpackages have been moved to their own repositories: [go-server-sdk-redis-redigo](https://github.com/launchdarkly/go-server-sdk-redis-redigo), [go-server-sdk-dynamodb](https://github.com/launchdarkly/go-server-sdk-dynamodb), and [go-server-sdk-consul](https://github.com/launchdarkly/go-server-sdk-consul). This removes the transitive dependencies on Redigo, AWS, etc.— which would otherwise be loaded for all Go modules that reference the SDK, even if you do not reference the subpackages that use them— and also allows fixes or new features to be released for those integrations without requiring a new SDK release.
- The configuration syntax for the database integrations has changed so that they now use a builder pattern, instead of a factory function with option arguments, and must be used in combination with [`ldcomponents.PersistentDataStore`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents#PersistentDataStore). See the new repositories for documentation and examples.
- The [`ldfiledata`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/ldfiledata) integration now also uses a builder pattern for configuration.
- Types related to [`LDClient.AllFlagsState()`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5#LDClient.AllFlagsState) are now in the [`interfaces/flagstate`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/flagstate) package, and their names have changed as follows: `FeatureFlagsState` is now `flagstate.AllFlags`; `ClientSideOnly` is now `flagstate.OptionClientSideOnly()`; `DetailsOnlyForTrackedFlags is now `flagstate.OptionDetailsOnlyForTrackedFlags()`.
- The component interfaces `FeatureStore` and `UpdateProcessor` have been renamed to `DataStore` and `DataSource`. The factory interfaces for these components now receive SDK configuration options in a different way that does not expose other components&#39; configurations to each other.
- The `PersistentDataStore` interface for creating your own database integrations has been simplified by moving all of the serialization and caching logic into the main SDK code.
- `FeatureFlag`, `Segment`, and other data model types are now in the package `gopkg.in/launchdarkly/go-server-sdk-evaluation.v1`. Application code will not normally need to refer to these types.
- All types related to the low-level handling of analytics events are now in the package `gopkg.in/launchdarkly/go-sdk-events.v1`. Application code will not normally need to refer to these types.

### Changed (requirements/dependencies/build):
- The lowest supported Go version is 1.14.
- Code coverage reports and benchmarks are now generated in every build. Unit test coverage of the entire SDK codebase has been greatly improved.

### Changed (behavioral changes):
- If analytics events are disabled, the SDK now avoids generating any analytics event objects internally. Previously they were created and then discarded, causing unnecessary heap churn.
- Network failures and server errors for streaming or polling requests were previously logged at `ERROR` level in most cases but sometimes at `WARN` level. They are now all at `WARN` level, but with a new behavior: if connection failures continue without a successful retry for a certain amount of time, the SDK will log a special `ERROR`-level message to warn you that this is not just a brief outage. The amount of time is one minute by default, but can be changed with the new [`LogDataSourceOutageAsErrorAfter`](https://pkg.go.dev/gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents#LoggingConfigurationBuilder.LogDataSourceOutageAsErrorAfter) option in `LoggingConfigurationBuilder`.
- Reading a `User` from JSON with `json.Unmarshal` now returns an error if the `key` property is missing or null.

### Changed (performance improvements):
- Improved the performance of flag evaluations when there is a very long user target list in a feature flag or user segment, by representing the user key collection internally as a map.
- Evaluation of rules involving regex matches, date/time values, and semantic versions, has been speeded up by pre-parsing the values in the rules. Also, parsing of date/time values and semantic versions in user attributes now uses a faster implementation and does not make any heap allocations.
- Evaluation of rules involving an equality match to multiple values (such as &#34;name is one of X, Y, Z&#34;) has been speeded up by converting the list of values to a map.
- Many internal methods have been rewritten to reduce the number of heap allocations in general.

### Removed:
- `DefaultConfig` was removed since it is no longer necessary: an empty `Config{}` is valid and will provide all of the documented default behavior. If you need to access the default value for a property, use the corresponding constant, such as `ldcomponents.DefaultEventsCapacity`.
- The `sharedtest` subpackage, which contains test helpers for the SDK itself, is now internal and cannot be used from application code. Test helpers that were meant to be public are now in the `testhelpers` subpackage.
- Removed all types, fields, and methods that were deprecated as of the most recent 4.x release.

## [4.17.3] - 2020-07-29
### Added:
- The SDK now recognizes a `clientSideAvailability` property which may be sent by LaunchDarkly services as an alternate way of indicating whether a flag is enabled for use by client-side JavaScript SDKs. This change does not introduce any new SDK functionality.

## [4.17.2] - 2020-05-13
### Fixed:
- Updated the version of [`go-yaml`](https://github.com/go-yaml/yaml) that is optionally used by the file data source feature, to version 2.3.0, due to a [vulnerability warning](https://vuln.whitesourcesoftware.com/vulnerability/CVE-2019-11254/) on version 2.2.1. Note that this is just the dependency version used when building the SDK; an application that uses the SDK may get different versions of its transitive dependencies depending on the package management system being used. The vulnerability would not affect normal usage of the SDK without the file data source, or with YAML files in the SDK&#39;s correct documented format.

## [4.17.1] - 2020-04-16
### Fixed:
- In streaming mode, a bug introduced in version 4.17.0 could cause a panic if the stream connection was broken and remained unavailable for a fairly long time (over half an hour).

## [4.17.0] - 2020-03-30
### Added:
- `Config` field `StreamInitialReconnectDelay` specifies how long the SDK should initially wait before retrying the stream connection after a failure. The default is one second; previously it was three seconds.

### Changed:
- When the SDK retries the stream connection after a failure, the delay now increases using an exponential backoff (which is reset to the initial level if the stream remains active for at least a minute), and each delay also has a random jitter from 0 to -50%. Previously, every retry used a three-second delay.

## [4.16.2] - 2020-03-13
### Added:
- CI tests now verify that the SDK supports Go 1.14.

### Fixed:
- In streaming mode, when using a persistent data store such as Redis, if the database was unavailable when the client initially started and made its first stream connection, a bug caused the SDK to give up on retrying and leave the client in a failed state. This has been fixed so that it will retry the stream connection once it detects that the database is available again (or, if using infinite caching mode, it will leave the same stream connection open and write the already-cached data to the database).

## [4.16.1] - 2020-02-10
### Changed:
- Diagnostic events reported by this SDK now have an SDK name of `go-server-sdk` instead of `Go`.


## [4.16.0] - 2020-02-04
This release introduces new types for building user properties and representing arbitrary JSON values. In the next major version, these will entirely replace the current deprecated equivalents.

### Added:
- `NewUserBuilder`, and its associated interfaces `UserBuilder` and `UserBuilderCanMakeAttributePrivate`. This is the new preferred mechanism for creating `User` instances when you need to set multiple properties; it reduces unsafe and inconvenient use of pointers.
- `User` property getter methods such as `GetName()`.
- The SDK has a new dependency on `gopkg.in/launchdarkly/go-sdk-common.v1`, which provides the helper types `ldvalue.Value` and `ldvalue.OptionalString`.
- In `LDClient`, `JSONVariation` and `JSONVariationDetail` are the new preferred mechanism for evaluating flags whose values can be of any JSON type. The value is represented as an `ldvalue.Value` rather than a `json.RawMessage`, but can be easily converted to `json.RawMessage` or to other Go types.
- In `LDClient`, `TrackData` and `TrackMetric` are the new preferred versions of `Track` and `TrackWithMetric`; they use `ldvalue.Value` rather than `interface{}` for the data parameter.
- `EvaluationReason` methods `GetRuleIndex()`, `GetRuleID()`, `GetPrerequisiteKey()`, `GetErrorKind()`. These were formerly only on concrete implementation types such as `EvaluationReasonRuleMatch`; they are being added to the interface type because in a future version, it will be changed to a struct.

### Fixed:
- By default, the SDK should log to `os.Stderr` with a minimum level of `ldlog.Info`, omitting only `Debug`-level messages. A bug introduced in 4.12.0 caused the default logger not to produce any output. It will now log at `Info` level by default again, as documented.
- When evaluating flags, the SDK incorrectly looked into the custom attributes for built-in attribute names if the built-in attribute was not set. This was inconsistent with LaunchDarkly flag evaluation on all other platforms, and has been fixed so that custom attributes using the names of built-in attributes are ignored. If you have been setting a built-in attribute as a custom attribute, please set the built-in attribute instead.

### Deprecated:
- All exported fields of `User`. In a future version, these will be hidden. Use getters such as `GetName()` to read these fields, and `NewUserBuilder` to set them.
- In `LDClient`, `JsonVariation`, `JsonVariationDetail`, `Track`, and `TrackWithMetric`. Use `JSONVariation`, `JSONVariationDetail`, `TrackData`, `TrackEvent`, or `TrackMetric` instead.
- The `EvaluationReason` implementation types such as `EvaluationReasonRuleMatch` are deprecated. Instead of casting to these types, use `EvaluationReason` methods such as `GetKind()` and `GetErrorKind()`.

## [4.15.0] - 2020-01-23
Note: if you are using the LaunchDarkly Relay Proxy to forward events, update the Relay to version 5.10.0 or later before updating to this Go SDK version.

### Added:
- The SDK now periodically sends diagnostic data to LaunchDarkly, describing the version and configuration of the SDK, the architecture and version of the runtime platform, and performance statistics. No credentials, hostnames, or other identifiable values are included. This behavior can be disabled with `Config.DiagnosticOptOut` or configured with `Config.DiagnosticRecordingInterval`.
- New `Config` fields `WrapperName` and `WrapperVersion` allow a library that uses the Go SDK to identify itself for usage data if desired.

## [4.14.2] - 2020-01-22
### Fixed:
- The SDK was logging a spurious "feature store query returned unexpected type" message at ERROR level if the application tried to evaluate an unknown feature flag when using a persistent data store.
- Added missing package comments.

## [4.14.1] - 2020-01-15
### Fixed:
- The SDK now specifies a uniquely identifiable request header when sending events to LaunchDarkly to ensure that events are only processed once, even if the SDK sends them two times due to a failed initial attempt.

## [4.14.0] - 2020-01-09
### Added:
- `ldhttp.ProxyOption`, for specifying an HTTP/HTTPS proxy URL programmatically rather than using environment variables.

### Fixed:
- `NewHTTPClientFactory` did not work correctly: it did construct an HTTP client, but did not actually apply any `ldhttp` transport options that were specified.
- In rare circumstances (depending on the exact data in the flag configuration, the flag's salt value, and the user properties), a percentage rollout could fail and return a default value, logging the error "Data inconsistency in feature flag ... variation/rollout object with no variation or rollout". This would happen if the user's hashed value fell exactly at the end of the last "bucket" (the last variation defined in the rollout). This has been fixed so that the user will get the last variation.

### Deprecated:
- Data model classes `FeatureFlag`, `Segment`, etc. are all deprecated and will be moved to another package in the future. Application code should never need to reference these types, and feature store integration code should only use abstractions like `ld.VersionedData`.
- `SegmentExplanation`, `HttpStatusError`, `ParseTime`, `ParseFloat64`, and `ToJsonRawMessage` were meant for internal use, and will be removed or made private in a future version.

## [4.13.1] - 2019-11-05
### Fixed:
- When using a persistent feature store (Redis, etc.), if multiple goroutines request the same flag in rapid succession when the flag data is not in the cache, the SDK will coalesce these requests so only a single database query is done.


## [4.13.0] - 2019-10-10
### Added:
- It is now possible to specify an infinite cache TTL for persistent feature stores by passing a negative number to the `CacheTTL` option, in which case the persistent store will never be read unless the application restarts. Use this mode with caution as described in the comments for `redis.CacheTTL`, `dynamodb.CacheTTL`, etc.

### Changed:
- When using a persistent store with an infinite cache TTL (see above), if the SDK receives a feature flag update from LaunchDarkly and is unable to write it to the persistent store because of a database outage, it will still update the data in the in-memory cache so it will be available to the application. This is different from the existing behavior when there is a finite cache TTL: in that case, if the database update fails, the in-memory cache will _not_ be updated because the update would be lost as soon as the cache expires.
- When using a persistent store, if there is a database error (indicating that the database may be unavailable, or at least that the most recent update did not get persisted), the SDK will continue to monitor the database availability. Once it returns to normal, if the cache TTL is finite, the SDK will restart the stream connection to ensure that it receives and persists a full set of flag data; if the cache TTL is infinite, it will assume the cache is up to date and will simply write it to the database.


## [4.12.0] - 2019-09-12
### Added:
- The Go SDK now has log levels, similar to the logging frameworks used in the other LaunchDarkly SDKs. Log messages can have a level of Debug, Info, Warn, or Error; by default, Debug is hidden. The new package [`ldlog`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v5/ldlog) defines these levels, and you can use `Config.Loggers.SetMinLevel()` and `Config.Loggers.SetBaseLogger()` to control the behavior. The old property `Config.Logger` still works but is deprecated.
- The SDK will produce very detailed output if you call `Config.Loggers.SetMinLevel(ldlog.Debug)`. This includes information about when and how it connects to LaunchDarkly, and a full dump of all analytics event data it is sending. Since the debug logging is very verbose, and the event data includes user properties, you should not normally enable this log level in production unless advised to by LaunchDarkly support.
- There is now a different property for specifying a feature store mechanism: `Config.FeatureStoreFactory`, which takes a factory method, rather than `Config.FeatureStore`, which takes an implementation instance. Using a factory method allows the implementation to access `Config` properties such as the logging configuration. The new methods `NewInMemoryFeatureStoreFactory`, `redis.NewRedisFeatureStoreFactory`, `consul.NewConsulFeatureStoreFactory`, and `dynamodb.NewDynamoDBFeatureStoreFactory` work with this mechanism.
- The SDK's CI build now verifies compatibility with Go 1.11 and 1.12.

### Deprecated:
- `Config.SamplingInterval`: the intended use case for the `SamplingInterval` feature was to reduce analytics event network usage in high-traffic applications. This feature is being deprecated in favor of summary counters, which are meant to track all events.
- `Config.Logger`: use `Config.Loggers` for more flexible configuration.
- `NewInMemoryFeatureStore`, `redis.NewRedisFeatureStoreWithDefault`, `consul.NewConsulFeaturStore`, `dynamodb.NewDynamoDBFeatureStore`: see above.


## [4.11.0] - 2019-08-19
### Added:
- Added support for upcoming LaunchDarkly experimentation features. See `LDClient.TrackWithMetric`.

## [4.10.0] - 2019-07-30
### Added:
- In the `redis` subpackage, the new option `DialOptions` allows configuration of any [connection option supported by Redigo](https://godoc.org/github.com/garyburd/redigo/redis#DialOption), such as setting a password or enabling TLS. (Thanks, [D-Raiser](https://github.com/launchdarkly/go-server-sdk/pull/8)!) Note that it was already possible to specify a password or TLS as part of the Redis URL.
- The new `Config` property `LogUserKeyInErrors`, if set to true, causes error log messages that are related to a specific user to include that user's key. This is false by default since user keys could be considered privileged information.

### Changed:
- If an error occurs during JSON serialization of user data in analytics events, previously all of the events that were going to be sent to LaunchDarkly at that point would be lost. Such an error could occur if a) the user's map of custom attributes was being modified by another goroutine, causing a concurrent modification panic, or b) a custom attribute value had a custom JSON marshaller that returned an error or caused a panic. The new behavior in these cases is that the SDK will log an error ("An error occurred while processing custom attributes ... the custom attributes for this user have been dropped from analytics data") and continue sending all of the event data except for the custom attributes for that user.

## [4.9.0] - 2019-07-23
### Added:
- The new `Config` property `LogEvaluationErrors`, if set to `true`, causes the client to output a `WARN:` log message whenever a flag cannot be evaluated because the flag does not exist, the user key was not specified, or the flag rules are invalid. The error message is the same as the message in the `error` object returned by the evaluation method. This may be useful in debugging if you are unexpectedly seeing default values instead of real values for a flag. Most of the other LaunchDarkly SDKs already log these messages by default, but since the Go SDK historically did not, this has been made an opt-in feature. It may be changed to be `true` by default in the next major version.
- The new [ldhttp](https://godoc.org/gopkg.in/launchdarkly/go-client.v4/ldhttp) package provides helper functions for setting custom HTTPS transport options, such as adding a root CA.
- The new [ldntlm](https://godoc.org/gopkg.in/launchdarkly/go-client.v4/ldntlm) package provides the ability to connect through a proxy server that uses NTLM authentication.

### Fixed:
- The SDK was not respecting the standard proxy server environment variable behavior (`HTTPS_PROXY`) that is normally provided by [`http.ProxyFromEnvironment`](https://godoc.org/net/http#ProxyFromEnvironment). (Thanks, [mightyguava](https://github.com/launchdarkly/go-server-sdk/pull/6)!)
- Under conditions where analytics events are being generated at an extremely high rate (for instance, if an application is evaluating a flag repeatedly in a tight loop on many goroutines), a thread could be blocked indefinitely within the Variation methods while waiting for the internal event processing logic to catch up with the backlog. The logic has been changed to drop events if necessary so application code will not be blocked (similar to how the SDK already drops events if the size of the event buffer is exceeded). If that happens, this warning message will be logged once: "Events are being produced faster than they can be processed; some events will be dropped". Under normal conditions this should never happen; this change is meant to avoid a concurrency bottleneck in applications that are already so busy that goroutine starvation is likely.

## [4.8.2] - 2019-07-02
### Added:
- Logging a message when failing to establish a streaming connection.

## [4.8.1] - 2019-06-12
### Fixed:
- A bug introduced in the 4.8.0 release was causing stream connections to restart frequently. ([#3](https://github.com/launchdarkly/go-server-sdk/issues/3))

## [4.8.0] - 2019-06-11
### Added:
- The `HTTPClientFactory` property in `Config` allows you to customize the HTTP client instances used by the SDK. This could be used, for instance, to support a type of proxy behavior that is not built into the Go standard library, or for compatibility with frameworks such as Google App Engine that require special networking configuration.

### Fixed:
- When using a custom attribute for rollout bucketing, the SDK now treats numeric values the same regardless of whether they are stored as `int` or `float64`, as long as the actual value is an integer. This is necessary to ensure consistent behavior because of the default behavior of JSON encoding in Go, which causes all numbers to become `float64` if they have been marshaled to JSON and then unmarshaled. As described in [the documentation for this feature](https://docs.launchdarkly.com/home/flags/targeting-users#percentage-rollouts), any floating-point value that has a fractional component is still disallowed.

## [4.7.4] - 2019-05-06
### Fixed:
- `Version` in `ldclient.go` is now correctly reported as `4.7.4`.


## [4.7.3] - 2019-04-29
### Changed:
- Import paths in subpackages and tests have been changed from `gopkg.in/launchdarkly/go-client.v4` to `gopkg.in/launchdarkly/go-server-sdk.v5`. Users of this SDK should update their import paths accordingly.
- This is the first release from the new `launchdarkly/go-server-sdk` repository.

## [4.7.2] - 2019-04-25
### Changed:
- The default value for the `Config` property `Capacity` (maximum number of events that can be stored at once) is now 10000, consistent with the other SDKs, rather than 1000.

### Fixed:
- If `Track` or `Identify` is called without a user, the SDK now will not send an analytics event to LaunchDarkly (since it would not be processed without a user).
- The size of the SDK codebase has been reduced considerably by eliminating unnecessary files from `vendor`.

### Note on future releases:
The LaunchDarkly SDK repositories are being renamed for consistency. All future releases of the Go SDK will use the name `go-server-sdk` rather than `go-client`. The import path will change to:

    "gopkg.in/launchdarkly/go-server-sdk.v5"

Since Go uses the repository name as part of the import path, to avoid breaking existing code, we will retain the existing `go-client` repository as well. However, it will not be updated after this release.

## [4.7.1] - 2019-01-09
### Fixed:
- Fixed a potential race condition in the DynamoDB and Consul feature store integrations where it might be possible to see a feature flag that depended on a prerequisite flag (or on a user segment) before the latter had been written to the store.

## [4.7.0] - 2018-12-18
### Added:
- The new configuration option `EventsEndpointUri` allows the entire URI for event posting to be customized, not just the base URI. This is used by the LaunchDarkly Relay Proxy and will not normally be needed by developers.
- Configuration options that did not have documentation comments are now documented.

## [4.6.1] - 2018-11-26
### Fixed:
- Fixed a bug in the DynamoDB feature store that caused read operations to fail sometimes if the `lddynamodb.Prefix` option was used.

## [4.6.0] - 2018-11-16
### Added:
- With the DynamoDB feature store, it is now possible to specify a prefix string for the database keys, so that multiple SDK clients can share the same DynamoDB table without interfering with each other's data as long as they use different prefixes. This feature was already available for Redis and Consul.

## [4.5.1] - 2018-11-15
### Fixed:
* Previously, the DynamoDB feature store implementation could fail if a feature flag contained an empty string in any property, since DynamoDB does not allow empty strings. This has been fixed by storing a JSON representation of the entire feature flag, rather than individual properties. The same implementation will be used by all other LaunchDarkly SDKs that provide a DynamoDB integration, so they will be interoperable.

## [4.5.0] - 2018-11-14
### Added:
- It is now possible to use DynamoDB or Consul as a persistent feature store, similar to the existing Redis integration. See the [`ldconsul`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v5/ldconsul) and [`lddynamodb`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v5/lddynamodb) subpackages, and the reference guide to ["Persistent data stores"](https://docs.launchdarkly.com/sdk/concepts/data-stores).

## [4.4.0] - 2018-10-30
### Added:
- It is now possible to inject feature flags into the client from local JSON or YAML files, replacing the normal LaunchDarkly connection. This would typically be for testing purposes. See the [`ldfiledata`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v5/ldfiledata) and [`ldfilewatch`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v5/ldfilewatch) subpackages.

- The `AllFlagsState` method now accepts a new option, `DetailsOnlyForTrackedFlags`, which reduces the size of the JSON representation of the flag state by omitting some metadata. Specifically, it omits any data that is normally used for generating detailed evaluation events if a flag does not have event tracking or debugging turned on.

### Fixed:
- JSON data from `AllFlagsState` is now slightly smaller even if you do not use the new option described above, because it completely omits the flag property for event tracking unless that property is true.

- Evaluating a prerequisite feature flag did not produce an analytics event if the prerequisite flag was off.

## [4.3.0] - 2018-08-27
### Added:
- The new `LDClient` method `AllFlagsState()` should be used instead of `AllFlags()` if you are passing flag data to the front end for use with the JavaScript SDK. It preserves some flag metadata that the front end requires in order to send analytics events correctly. Versions 2.5.0 and above of the JavaScript SDK are able to use this metadata, but the output of `AllFlagsState()` will still work with older versions.
- The `AllFlagsState()` method also allows you to select only client-side-enabled flags to pass to the front end, by using the option `ClientSideOnly`.
- The new `LDClient` methods `BoolVariationDetail`, `IntVariationDetail`, `Float64VariationDetail`, `StringVariationDetail`, and `JsonVariationDetail` allow you to evaluate a feature flag (using the same parameters as you would for `BoolVariation`, etc.) and receive more information about how the value was calculated. This information is returned in an `EvaluationDetail` object, which contains both the result value and an `EvaluationReason` which will tell you, for instance, if the user was individually targeted for the flag or was matched by one of the flag's rules, or if the flag returned the default value due to an error.

### Deprecated:
- `LDClient.AllFlags()`, `EvalResult`, `FeatureFlag.Evaluate`, `FeatureFlag.EvaluateExplain`

## [4.2.2] - 2018-08-03
### Fixed:
- Fixed a bug that caused a panic if an I/O error occurred while reading the response body for a polling request.
- Fixed a bug that caused a panic if a prerequisite feature flag evaluated to a non-scalar value (array or map/hash).
- Receiving an HTTP 400 error from LaunchDarkly should not make the client give up on sending any more requests to LaunchDarkly (unlike a 401 or 403).

## [4.2.1] - 2018-06-27
### Fixed:
- Polling processor regressed to polling only once in release 4.1.0.  This has been fixed.



## [4.2.0] - 2018-06-26
### Changed:
- The client now treats most HTTP 4xx errors as unrecoverable: that is, after receiving such an error, it will not make any more HTTP requests for the lifetime of the client instance, in effect taking the client offline. This is because such errors indicate either a configuration problem (invalid SDK key) or a bug, which is not likely to resolve without a restart or an upgrade. This does not apply if the error is 400, 408, 429, or any 5xx error.

## [4.1.0] - 2018-06-14
### Changed

The Go client now depends on the latest release of 1.0.0 of LaunchDarkly fork of eventsource, which supports the Close() method.

### Fixed

- Calling Close on the client now immediately closes the streaming connection, if the client is in streaming mode.
- During initialization, if the client receives a 401 error from LaunchDarkly (indicating an invalid SDK key), the client constructor will return immediately rather than waiting for a timeout, since there is no way for the client to recover if the SDK key is wrong. The Initialized() method will return false in this case.
- More generally, the error response for creating a client will also indicate that initialization has failed if the client has not yet been initialized by the UpdateProcessor.

## [4.0.0] - 2018-05-10

### Changed
- To reduce the network bandwidth used for analytics events, feature request events are now sent as counters rather than individual events, and user details are now sent only at intervals rather than in each event. These behaviors can be modified through the LaunchDarkly UI and with the new configuration option `InlineUsersInEvents`. For more details, read [Data Export](https://docs.launchdarkly.com/home/data-export).
- When sending analytics events, if there is a connection error or an HTTP 5xx response, the client will try to send the events again one more time after a one-second delay.
- The `Close` method on the client now conforms to the `io.Closer` interface.

### Added
- The new global `VersionedDataKinds` is an array of all existing `VersionedDataKind` instances. This is mainly useful if you are writing a custom `FeatureStore` implementation. (Thanks, [mlafeldt](https://github.com/launchdarkly/go-client/pull/117)!)


## [3.1.0] - 2018-03-19
### Added
- Convenience functions `NewUser` and `NewAnonymousUser`, for creating a user struct given only the key. (Thanks, [mlafeldt](https://github.com/launchdarkly/go-client/pull/109)!)
### Fixed
- In the Redis feature store, fixed a synchronization problem that could cause a feature flag update to be missed if several of them happened in rapid succession.
- Fixed errors in the Readme example code. (Thanks, [mlafeldt](https://github.com/launchdarkly/go-client/pull/110)!)

## [3.0.0] - 2018-02-19

### Added
- Support for a new LaunchDarkly feature: reusable user segments.
- The mechanism by which the client retrieves feature and segment data from the server is now customizable through an interface, `UpdateProcessor`. This will be used in future to support test fixtures.

### Changed
- The `FeatureStore` interface has been changed to support user segment data as well as feature flags. Existing code that uses `InMemoryFeatureStore` or `RedisFeatureStore` should work as before, but custom feature store implementations will need to be updated.
- Logging is now done through an interface, `Logger`, instead of directly referencing `log.Logger`. Existing code that uses `log.Logger` should still work as before.



## [2.3.0] - 2018-01-31

### Changed
- When evaluating a feature flag, if the client has not yet fully initialized but you are using a Redis store that has already been populated, the client will now use the last known feature data from Redis rather than returning a default value.
- In polling mode, the minimum polling interval is now 30 seconds. Smaller configured values will be adjusted up to the minimum.
- The streaming client will no longer reconnect after detecting an invalidated SDK key.
- Added a build tag, `launchdarkly_no_redis`, which allows building without the Redis dependency.

### Fixed
- Fixed a bug where a previously deleted feature flag might be considered still available.


## [2.2.3] - 2017-12-21

### Added

- Allow user to stop user attributes from being sent in analytics events back to LaunchDarkly.  Set `PrivateAttributeNames` to a list of attributes to avoid sending, or set `AllAttributesPrivate` to `true` to send no attributes.

### Changed

- Accept an interface for the `Logger` configuration option (thanks @ZiaoGeorgeJiang).

## [2.1.0] - 2017-11-16

### Added
- Stop processing streaming events and errors after `Close()`.


## [2.0.0] - 2016-08-08
### Added
- Support for multivariate feature flags. New methods `StringVariation`, `JsonVariation` and `IntVariation` and `Float64Variation` for multivariates.
- New `AllFlags` method returns all flag values for a specified user.
- New `SecureModeHash` function computes a hash suitable for the new LaunchDarkly JavaScript client's secure mode feature.

### Changed
- The `Feature` data model has been replaced with `FeatureFlag`.

### Deprecated
- The `Toggle` call has been deprecated in favor of `BoolVariation`.
