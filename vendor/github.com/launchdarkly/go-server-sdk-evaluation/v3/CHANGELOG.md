# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [3.0.1] - 2024-08-28
### Changed:
- Bumped launchdarkly/go-sdk-common from v3.0.0 to v3.1.0
- Bumped launchdarkly/go-test-helpers from v3.0.1 to v3.0.2
- Bumped mailru/easyjson from v0.7.6 to v0.7.7
- Bumped stretchr/testify from v1.7.0 to v1.9.0


### Fixed:
- Bumped launchdarkly/go-semver from v1.0.2 to v1.0.3, which removes the dependency on the "testing" package

## [3.0.0] - 2023-10-11
### Added:
- The flag model now supports new summary exclusion, sampling, and migration related properties.
- Each new flag property is modifiable through the flag builder interface.
- The `PrerequisiteFlagEvent` also has a new property to support flag summary exclusions.

## [2.0.2] - 2023-03-01
### Changed:
- Bumped go-sdk-common to v3.0.1.

## [2.0.1] - 2022-12-01
### Fixed:
- Fixed a linter error. There are no functional changes.

## [2.0.0] - 2022-11-30
This major version release of `go-server-sdk-evaluation` corresponds to the upcoming v6.0.0 release of the LaunchDarkly Go SDK (`go-server-sdk`), and cannot be used with earlier SDK versions. As before, this package is intended for internal use by the Go SDK, and by LaunchDarkly services; other use is unsupported.

### Added:
- The data model types in `ldmodel` now include optional properties related to new context features, such as `contextKind`.
- The evaluation engine now includes logic for using the new data model properties in evaluations.
- `EvaluatorOptionEnableSecondaryKey` enables an opt-in behavior to enable the obsolete `secondary` user attribute in rollouts. This is only for use by internal LaunchDarkly service code that processes user data from older SDKs.

### Changed:
- The minimum Go version is now 1.18.
- The package now uses a regular import path (`github.com/launchdarkly/go-server-sdk-evaluation/v2`) rather than a `gopkg.in` path (`gopkg.in/launchdarkly/go-server-sdk-evaluation.v1`).
- The dependency on `gopkg.in/launchdarkly/go-sdk-common.v2` has been changed to `github.com/launchdarkly/go-sdk-common/v3`.
- The evaluation engine now operates on the `ldcontext.Context` type rather than `lduser.User`.
- Evaluator now returns a new `Result` type that includes more information for use in analytics events.
- `SegmentRule.Weight` is now an `OptionalInt`, instead of an `int` that uses a negative value to mean "undefined".

### Removed:
- In `ldmodel` data model types, removed methods such as `GetKey()` whose purpose was to implement interfaces in other packages that have been removed.
- Removed `FeatureFlag.IsExperimentationEnabled()`, which has been superseded by the `Result` type.
- Removed all symbols that were deprecated as of the latest v1 release.

## [1.5.0] - 2022-01-05
### Added:
- `NewEvaluatorWithOptions`, `EvaluatorOptionBigSegmentProvider`, `EvaluatorOptionErrorLogger`
- If a logger is specified with `EvaluatorOptionErrorLogger`, the evaluator will now log error messages in all cases involving a `MALFORMED_FLAG`, to explain more specifically what is wrong with the flag data.

### Fixed:
- It is no longer possible for the evaluator to recurse indefinitely due to a circular reference in flag prerequisites. Now it will stop the evaluation if it detects a circular reference, and return a `MALFORMED_FLAG` error reason.

### Deprecated:
- `NewEvaluatorWithBigSegments`

## [1.4.1] - 2021-08-20
### Fixed:
- When using big segments, if a big segment store query for a user returned `nil`, the evaluator was treating that as an automatic exclusion for the user and skipping any rules that might exist in the segment. It should instead treat `nil` the same as an empty result.

## [1.4.0] - 2021-07-19
### Added:
- Added support for evaluating big segments.

## [1.3.0] - 2021-06-17
### Added:
- The SDK now supports the ability to control the proportion of traffic allocation to an experiment. This works in conjunction with a new platform feature now available to early access customers.

## [1.2.2] - 2021-06-03
### Fixed:
- The 1.2.1 release updated the `go-sdk-common` dependency, but not to the latest version that actually contained the relevant bugfix. This release updates to the latest.

## [1.2.1] - 2021-06-03
### Fixed:
- Updated `go-jsonstream` to [v1.0.1](https://github.com/launchdarkly/go-jsonstream/releases/tag/1.0.1) to incorporate a bugfix in JSON number parsing.

## [1.2.0] - 2021-02-26
### Added:
- New `Generation` field in `ldmodel.Segment`.

## [1.1.2] - 2021-02-11
### Fixed:
- When deserializing feature flags from JSON, an explicit null value for the `rollout` property (as opposed to just omitting the property) was being treated as an error. The LaunchDarkly service endpoints do not ever send `rollout: null`, but it should be considered valid if encountered in JSON from some other source.

## [1.1.1] - 2021-01-20
### Fixed:
- When using semantic version operators, semantic version strings were being rejected by the parser if they contained a zero digit in any position _after_ the first character of a numeric version component. For instance, `0.1.2` and `1.2.3` were accepted, and `01.2.3` was correctly rejected (leading zeroes for nonzero values are not allowed), but `10.2.3` was incorrectly rejected.

## [1.1.0] - 2020-12-17
### Added:
- In `ldmodel`, there are now additional JSON marshaling and unmarshaling methods that can interact directly with `go-jsonstream` writers and readers (see below).
- You can now get automatic integration with EasyJSON by setting the build tag `launchdarkly_easyjson`, which causes `MarshalEasyJSON` and `UnmarshalEasyJSON` methods to be added to `FeatureFlag` and `Segment`.

### Changed:
- The internal JSON serialization logic now uses [`go-jsonstream`](https://github.com/launchdarkly/go-jsonstream) instead of the deprecated `go-sdk-common.v2/jsonstream`.
- The internal JSON deserialization logic, which previously used `encoding/json`, now uses `go-jsonstream` for a considerable increase in efficiency.

## [1.0.1] - 2020-10-08
### Fixed:
- When serializing flags and segments to JSON, properties with default values (such as false booleans or empty arrays) were being dropped entirely to save bandwidth. However, these representations may be consumed by SDKs other than the Go SDK, and some of the LaunchDarkly SDKs do not tolerate missing properties, so this has been fixed to remain consistent with the less efficient behavior of Go SDK 4.x.

## [1.0.0] - 2020-09-18
Initial release of this flag evaluation support code that will be used with versions 5.0.0 and above of the LaunchDarkly Server-Side SDK for Go.
