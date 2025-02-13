[![GoDoc](https://godoc.org/github.com/launchdarkly/eventsource?status.svg)](http://godoc.org/github.com/launchdarkly/eventsource)
[![Actions Status](https://github.com/launchdarkly/eventsource/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/launchdarkly/eventsource/actions/workflows/ci.yml)

# Eventsource

Eventsource implements a [Go](http://golang.org/) implementation of client and server to allow streaming data one-way over a HTTP connection using the Server-Sent Events API http://dev.w3.org/html5/eventsource/

This is a fork of: https://github.com/donovanhide/eventsource

## Supported Go versions

The library supports the 'latest' and 'penultimate' Go versions defined in [this file](./.github/variables/go-versions.env).

LaunchDarkly intends to keep those versions up-to-date with the Go project's latest two releases, which represents a support
period of roughly 1 year. This policy is intended to match Go's official [Release Policy](https://go.dev/doc/devel/release):
each major Go release is supported until there are two newer major releases.

Additionally, a 'minimum' version is tested in CI but not officially supported. This minimum version is found in [go.mod](./go.mod).
This version may be [bumped](./CONTRIBUTING.md#bumping-the-minimum-go-version) from time to time as new Go features
become available that are useful to the SDK.

## Installation

    go get github.com/launchdarkly/eventsource

## Documentation

* [Reference](http://godoc.org/github.com/launchdarkly/eventsource)

## License

Eventsource is available under the [Apache License, Version 2.0](http://www.apache.org/licenses/LICENSE-2.0.html).
