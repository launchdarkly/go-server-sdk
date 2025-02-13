# CCache

This fork of the original [CCache](https://github.com/karlseguin/ccache), a concurrent LRU cache written in Go, exists only to support the build requirements of the LaunchDarkly Go SDK. Changes in this fork should _not_ be submitted upstream.

Specifically, the issue is that the LaunchDarkly Go SDK (in major versions up to and including v5) explicitly supports use by applications that use non-module-compatible package managers such as `dep` instead of Go modules. This means that there cannot be any dependencies that are modules with a major version greater than 1, because they would have a `/vN` major version suffix in their import paths, which `dep` and similar tools do not understand. This fork simply removes the `/v2` path suffix from `ccache` and resets the major version to 1.

The versioning of this package starts at v1.1.0, to avoid confusion with the versioning of the original repository which had 1.0.0 and 1.0.1 releases.

For all other information about this package, see the original repository.
