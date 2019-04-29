# Change log

All notable changes to the LaunchDarkly Go SDK will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [4.7.3] - 2019-04-29
### Changed:
- Import paths in subpackages and tests have been changed from `gopkg.in/launchdarkly/go-client.v4` to `gopkg.in/launchdarkly/go-server-sdk.v4`. Users of this SDK should update their import paths accordingly.
- This is the first release from the new `launchdarkly/go-server-sdk` repository.

## [4.7.2] - 2019-04-25
### Changed:
- The default value for the `Config` property `Capacity` (maximum number of events that can be stored at once) is now 10000, consistent with the other SDKs, rather than 1000.

### Fixed:
- If `Track` or `Identify` is called without a user, the SDK now will not send an analytics event to LaunchDarkly (since it would not be processed without a user).
- The size of the SDK codebase has been reduced considerably by eliminating unnecessary files from `vendor`.

## Note on future releases
The LaunchDarkly SDK repositories are being renamed for consistency. All future releases of the Go SDK will use the name `go-server-sdk` rather than `go-client`. The import path will change to:

    "gopkg.in/launchdarkly/go-server-sdk.v4"

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
- It is now possible to use DynamoDB or Consul as a persistent feature store, similar to the existing Redis integration. See the [`ldconsul`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v4/ldconsul) and [`lddynamodb`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v4/lddynamodb) subpackages, and the reference guide to ["Using a persistent feature store"](https://docs.launchdarkly.com/v2.0/docs/using-a-persistent-feature-store).

## [4.4.0] - 2018-10-30
### Added:
- It is now possible to inject feature flags into the client from local JSON or YAML files, replacing the normal LaunchDarkly connection. This would typically be for testing purposes. See the [`ldfiledata`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v4/ldfiledata) and [`ldfilewatch`](https://godoc.org/gopkg.in/launchdarkly/go-server-sdk.v4/ldfilewatch) subpackages.

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
- To reduce the network bandwidth used for analytics events, feature request events are now sent as counters rather than individual events, and user details are now sent only at intervals rather than in each event. These behaviors can be modified through the LaunchDarkly UI and with the new configuration option `InlineUsersInEvents`. For more details, see [Analytics Data Stream Reference](https://docs.launchdarkly.com/v2.0/docs/analytics-data-stream-reference).
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
