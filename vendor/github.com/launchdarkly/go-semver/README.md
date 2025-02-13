# LaunchDarkly Semantic Version Package

[![Circle CI](https://circleci.com/gh/launchdarkly/go-semver.svg?style=shield)](https://circleci.com/gh/launchdarkly/go-semver) [![Documentation](https://godoc.org/github.com/launchdarkly/go-semver?status.svg)](https://godoc.org/github.com/launchdarkly/go-semver)

## Overview

This Go package implements parsing and comparison of semantic version (semver) strings, as defined by the [Semantic Versioning 2.0.0 specification](https://semver.org/).

Several semver implementations exist for Go. This implementation was designed for high performance in applications where semver operations may be done frequently, such as in the [LaunchDarkly Go SDK](https://github.com/launchdarkly/go-server-sdk). To that end, it does not use regular expressions and it never allocates data on the heap.

It does not include any additional functionality beyond what is defined in the Semantic Versioning 2.0.0 specification, such as comparison against range/wildcard expressions like ">=1.0.0" or "2.5.x".

This package has no external dependencies other than the regular Go runtime.

## Supported Go versions

The library supports the 'latest' and 'penultimate' Go versions defined in [this file](./.github/variables/go-versions.env).

LaunchDarkly intends to keep those versions up-to-date with the Go project's latest two releases, which represents a support
period of roughly 1 year. This policy is intended to match Go's official [Release Policy](https://go.dev/doc/devel/release):
each major Go release is supported until there are two newer major releases.

Additionally, a 'minimum' version is tested in CI but not officially supported. This minimum version is found in [go.mod](./go.mod).
This version may be [bumped](./CONTRIBUTING.md#bumping-the-minimum-go-version) from time to time as new Go features
become available that are useful to the SDK.

## Contributing

We encourage pull requests and other contributions from the community. Check out our [contributing guidelines](CONTRIBUTING.md) for instructions on how to contribute to this project.
