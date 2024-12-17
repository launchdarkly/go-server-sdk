# Changelog

## [0.4.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.3.0...ldai/v0.4.0) (2024-12-17)


### Features

* Add `TrackError` to mirror `TrackSuccess` ([#225](https://github.com/launchdarkly/go-server-sdk/issues/225)) ([ccd2c64](https://github.com/launchdarkly/go-server-sdk/commit/ccd2c644efdfd4de12ce0bff786e7f6b6764b153))

## [0.3.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.2.0...ldai/v0.3.0) (2024-12-10)


### ⚠ BREAKING CHANGES

* Rename versionKey to variationKey ([#221](https://github.com/launchdarkly/go-server-sdk/issues/221))

### Code Refactoring

* Rename versionKey to variationKey ([#221](https://github.com/launchdarkly/go-server-sdk/issues/221)) ([470f8c1](https://github.com/launchdarkly/go-server-sdk/commit/470f8c1f90022abb102f6fcf0c2856297baa8842))

## [0.2.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.1.1...ldai/v0.2.0) (2024-12-09)


### ⚠ BREAKING CHANGES

* Rename model and provider id to name ([#218](https://github.com/launchdarkly/go-server-sdk/issues/218))

### Bug Fixes

* propagate parsed model params into returned AI config ([#220](https://github.com/launchdarkly/go-server-sdk/issues/220)) ([f75b7a8](https://github.com/launchdarkly/go-server-sdk/commit/f75b7a8df5f5e62f544d5e95cbc75bd82352ca57))


### Code Refactoring

* Rename model and provider id to name ([#218](https://github.com/launchdarkly/go-server-sdk/issues/218)) ([ebdc281](https://github.com/launchdarkly/go-server-sdk/commit/ebdc281c667446e04b37ef44ee5e3f953d288eb2))

## [0.1.1](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.1.0...ldai/v0.1.1) (2024-12-06)


### Bug Fixes

* add prefix to Feedback constants ([#216](https://github.com/launchdarkly/go-server-sdk/issues/216)) ([67d3564](https://github.com/launchdarkly/go-server-sdk/commit/67d35649f9ae32541ba37b47473855a9ac8cb52b))

## 0.1.0 (2024-12-05)


### Features

* pass tracker's config into TrackRequest ([#210](https://github.com/launchdarkly/go-server-sdk/issues/210)) ([8321db6](https://github.com/launchdarkly/go-server-sdk/commit/8321db64849214bc17860993a392f7c19385c945))


### Bug Fixes

* pass parsed versionKey to Tracker ([#211](https://github.com/launchdarkly/go-server-sdk/issues/211)) ([215116a](https://github.com/launchdarkly/go-server-sdk/commit/215116a2de760e88be9b14773856edc99fa9dda3))
