# Contributing to the LaunchDarkly Server-side SDK for Go

LaunchDarkly has published an [SDK contributor's guide](https://docs.launchdarkly.com/sdk/concepts/contributors-guide) that provides a detailed explanation of how our SDKs work. See below for additional information on how to contribute to this SDK.

## Submitting bug reports and feature requests

The LaunchDarkly SDK team monitors the [issue tracker](https://github.com/launchdarkly/go-server-sdk/issues) in the SDK repository. Bug reports and feature requests specific to this SDK should be filed in this issue tracker. The SDK team will respond to all newly filed issues within two business days.

## Submitting pull requests

We encourage pull requests and other contributions from the community. Before submitting pull requests, ensure that all temporary or unintended code is removed. Don't worry about adding reviewers to the pull request; the LaunchDarkly SDK team will add themselves. The SDK team will acknowledge all pull requests within two business days.

## Build instructions

### Prerequisites

This project should be built against the lowest supported Go version as described in [README.md](./README.md).

### Bumping the Minimum Go Version

The SDK is tested against three Go versions: the latest, penultimate, and a minimum based on the SDKs usage of Go features.

Whereas the latest and penultimate are updates on a regular cadence to track upstream Go releases, the minimum version
may be bumped at the discretion of the SDK maintainers to take advantage of new features.

Invoke the following make command, which will update `go.mod`, `ldotel/go.mod`, `testservice/go.mod`, and 
`.github/variables/go-versions.env` (pass the desired Go version):
```shell
make bump-min-go-version MIN_GO_VERSION=1.18
```

### Building

To build all modules without running any tests:
```shell
make
```

To run the linter:
```shell
make lint
```

### Testing

To build all modules and run all unit tests:
```shell
make test
```

### Clean

To clean temporary files created by other targets:
```shell
make clean
```

### Working Cross Module

If you have a change which affects more than a single module, then you can use a go workspace.

You can create a workspace using:
```shell
make workspace
```

## Coding best practices

The Go SDK can be used in high-traffic application/service code where performance is critical. There are a number of coding principles to keep in mind for maximizing performance. The benchmarks that are run in CI are helpful in measuring the impact of code changes in this regard.

### Avoid heap allocations

Go's memory model uses a mix of stack and heap allocations, with the compiler transparently choosing the most appropriate strategy based on various type and scope rules. It is always preferable, when possible, to keep ephemeral values on the stack rather than on the heap to avoid creating extra work for the garbage collector.

- The most obvious rule is that anything explicitly allocated by reference (`x := &SomeType{}`), or returned by reference (`return &x`), will be allocated on the heap. Avoid this unless the object has mutable state that must be shared.
- Casting a value type to an interface causes it to be allocated on the heap, since an interface is really a combination of a type identifier and a hidden pointer.
- A closure that references any variables outside of its scope (including the method receiver, if it is inside a method) causes an object to be allocated on the heap containing the values or addresses of those variables.
- Treating a method as an anonymous function (`myFunc := someReceiver.SomeMethod`) is equivalent to a closure.

Allocations are counted in the benchmark output: "5 allocs/op" means that a total of 5 heap objects were allocated during each run of the benchmark. This does not mean that the objects were retained, only that they were allocated at some point.

For any methods that should be guaranteed _not_ to do any heap allocations, the corresponding benchmarks should have names ending in `NoAlloc`. The `make benchmarks` target will automatically fail if allocations are detected in any benchmarks that have this name suffix.

For a much (MUCH) more detailed breakdown of this behavior, you may use the option `GODEBUG=allocfreetrace=1` while running a unit test or benchmark. This provides the type and code location of literally every heap allocation during the run. The output is extremely verbose, so it is recommended that you:

1. use the Makefile helper `benchmark-allocs` (see below) to reduce the number of benchmark runs and avoid capturing allocations from the Go tools themselves;
2. search the stacktrace output to find the method you are actually testing (such as `BoolVariation`) rather than the benchmark function name, so you are not looking at actions that are just part of the benchmark setup;
3. consider writing a smaller temporary benchmark specifically for this purpose, since most of the existing benchmarks will iterate over a series of parameters.

```bash
BENCHMARK=BenchmarkMySampleOperation make benchmark-allocs
```

### Minimize overhead when logging at debug level

The `Debug` logging level can be a useful diagnostic tool, and it is OK to log very verbosely at this level. However, to avoid slowing things down in the usual case where this log level is disabled, keep in mind:

1. Avoid string concatenation; use `Printf`-style placeholders instead.
2. Before calling `loggers.Debug` or `loggers.Debugf`, check `loggers.IsDebugEnabled()`. If it returns false, you should skip the `Debug` or `Debugf` call, since otherwise it can incur some overhead from converting the parameters of that call to `interface{}` values.

### Test coverage

It is important to keep unit test coverage as close to 100% as possible in this project. You can view the latest code coverage report in CircleCI, as `coverage.html` and `coverage.txt` in the artifacts for the latest Go version build. You can also generate this information locally with `make test-coverage`.

The build will fail if there are any uncovered blocks of code, unless you explicitly add an override by placing a comment that starts with `// COVERAGE` somewhere within that block. Sometimes a gap in coverage is unavoidable, usually because the compiler requires us to provide a code path for some condition that in practice can't happen and can't be tested. Exclude these paths with a `// COVERAGE` comment.
