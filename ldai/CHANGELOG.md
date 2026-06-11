# Changelog

## [0.9.2](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.9.1...ldai/v0.9.2) (2026-06-11)


### Bug Fixes

* **deps:** revert root module, sub-modules, and testservice to v3 core libraries ([#392](https://github.com/launchdarkly/go-server-sdk/issues/392)) ([0c0b0e8](https://github.com/launchdarkly/go-server-sdk/commit/0c0b0e81d5ed9d34c07d59d8a69e94e055e9675c))

## [0.9.1](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.9.0...ldai/v0.9.1) (2026-06-04)


### Bug Fixes

* update sub-modules to v4 deps ([#382](https://github.com/launchdarkly/go-server-sdk/issues/382)) ([d772d5b](https://github.com/launchdarkly/go-server-sdk/commit/d772d5bdac5b66d12cdfbeb85e09e93f8c71defb))

## [0.9.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.8.1...ldai/v0.9.0) (2026-05-11)


### ⚠ BREAKING CHANGES

* Tracker no longer returned alongside AI Configs, use Config.CreateTracker() instead
* Add per-execution runId, at-most-once tracking, and cross-process tracker resumption ([#363](https://github.com/launchdarkly/go-server-sdk/issues/363))

### Features

* Add per-execution runId, at-most-once tracking, and cross-process tracker resumption ([#363](https://github.com/launchdarkly/go-server-sdk/issues/363)) ([c11294f](https://github.com/launchdarkly/go-server-sdk/commit/c11294f4a5de402317aca9276e48a4094161adf7))
* Rename TrackUsage to TrackTokens ([#364](https://github.com/launchdarkly/go-server-sdk/issues/364)) ([9b0863a](https://github.com/launchdarkly/go-server-sdk/commit/9b0863a0d7ab1c2dd366bebd1f916ba9eb4a7ad0))
* Tracker no longer returned alongside AI Configs, use Config.CreateTracker() instead ([c11294f](https://github.com/launchdarkly/go-server-sdk/commit/c11294f4a5de402317aca9276e48a4094161adf7))


### Bug Fixes

* Prevent context attributes from influencing judge template parsing (SEC-8020) ([#361](https://github.com/launchdarkly/go-server-sdk/issues/361)) ([a14fc86](https://github.com/launchdarkly/go-server-sdk/commit/a14fc86e64c8f2e6555e7ece4ad08081d46c2067))

## [0.8.1](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.8.0...ldai/v0.8.1) (2026-02-26)


### Bug Fixes

* Improve usage reporting ([#353](https://github.com/launchdarkly/go-server-sdk/issues/353)) ([0146f76](https://github.com/launchdarkly/go-server-sdk/commit/0146f762af466f7ab7d997bbd57135eb1fcb0930))

## [0.8.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.7.2...ldai/v0.8.0) (2026-02-10)


### Features

* Add AI Config judge support ([#345](https://github.com/launchdarkly/go-server-sdk/issues/345)) ([4a9d03d](https://github.com/launchdarkly/go-server-sdk/commit/4a9d03d947147eff2506adc3aa0e1322ce4fa3d9))

## [0.7.2](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.7.1...ldai/v0.7.2) (2025-09-02)


### Bug Fixes

* Add usage tracking to config method ([#307](https://github.com/launchdarkly/go-server-sdk/issues/307)) ([400a61e](https://github.com/launchdarkly/go-server-sdk/commit/400a61ea733a004dbedc1bbcaf6f897f3988f40d))

## [0.7.1](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.7.0...ldai/v0.7.1) (2025-07-29)


### Bug Fixes

* Remove deprecated track generation event ([#301](https://github.com/launchdarkly/go-server-sdk/issues/301)) ([ca20f09](https://github.com/launchdarkly/go-server-sdk/commit/ca20f092cbc3098e0a4b98041d6f7147c0b2cf5a))
* Update AI tracker to include model & provider name for metrics generation ([#302](https://github.com/launchdarkly/go-server-sdk/issues/302)) ([9a84146](https://github.com/launchdarkly/go-server-sdk/commit/9a84146c755a6abfb2b64eafcfed8f06242128fe))

## [0.7.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.6.0...ldai/v0.7.0) (2025-05-19)


### Features

* Add GetSummary method to tracker ([#269](https://github.com/launchdarkly/go-server-sdk/issues/269)) ([5cd7b46](https://github.com/launchdarkly/go-server-sdk/commit/5cd7b463fe641cdc4dff3dbce1a68416c4208b6c))

## [0.6.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.5.0...ldai/v0.6.0) (2025-02-07)


### Features

* Add variation version to metric data ([#244](https://github.com/launchdarkly/go-server-sdk/issues/244)) ([01ee033](https://github.com/launchdarkly/go-server-sdk/commit/01ee0339c9f1adad462c1dba6927a2c213f5e032))

## [0.5.0](https://github.com/launchdarkly/go-server-sdk/compare/ldai/v0.4.0...ldai/v0.5.0) (2025-01-23)


### Features

* Add timeToFirstToken to Tracker ([234b171](https://github.com/launchdarkly/go-server-sdk/commit/234b1716a7c09400f850af8e6c3c805498fc7321))
* Drop Set method on Metric interface. ([234b171](https://github.com/launchdarkly/go-server-sdk/commit/234b1716a7c09400f850af8e6c3c805498fc7321))
* Update minimum go version to 1.20. ([46c9694](https://github.com/launchdarkly/go-server-sdk/commit/46c9694e733356cf4d051e7b72241b0a6e330a37))

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

* propagate parsed model params into returned AI Config ([#220](https://github.com/launchdarkly/go-server-sdk/issues/220)) ([f75b7a8](https://github.com/launchdarkly/go-server-sdk/commit/f75b7a8df5f5e62f544d5e95cbc75bd82352ca57))


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
