# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [1.0.3](https://github.com/launchdarkly/go-semver/compare/1.0.2...v1.0.3) (2024-08-21)


### Bug Fixes

* **deps:** bump supported Go versions to 1.23 and 1.22 ([960049e](https://github.com/launchdarkly/go-semver/commit/960049ef7fd30761cb931bcf6813bbb7ca21fd31))
* don't reference testing package from main module ([#11](https://github.com/launchdarkly/go-semver/issues/11)) ([272fb9c](https://github.com/launchdarkly/go-semver/commit/272fb9cb6a6b854ba94f09edc2590fafdc149e32))

## [1.0.2] - 2021-01-20
### Fixed:
- Valid semantic version strings were being rejected by the parser if they contained a zero digit in any position _after_ the first character of a numeric version component. For instance, &#34;0.1.2&#34; and &#34;1.2.3&#34; were accepted, and &#34;01.2.3&#34; was correctly rejected (leading zeroes for nonzero values are not allowed), but &#34;10.2.3&#34; was incorrectly rejected.

## [1.0.1] - 2020-09-18
### Fixed:
- Removed some unwanted package dependencies.
- Added CI build for Go 1.15 and removed build for 1.13; Go 1.13 is now EOL.

## [1.0.0] - 2020-06-15
Initial release.
