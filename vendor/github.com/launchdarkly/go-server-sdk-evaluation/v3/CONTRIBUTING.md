# Contributing to this project
 
LaunchDarkly has published an [SDK contributor's guide](https://docs.launchdarkly.com/docs/sdk-contributors-guide) that provides a detailed explanation of how our SDKs work. See below for additional information on how to contribute to this project.
 
## Submitting bug reports and feature requests

The LaunchDarkly SDK team monitors the [issue tracker](https://github.com/launchdarkly/go-server-sdk-evaluation/issues) in tis repository. Bug reports and feature requests specific to this project should be filed in this issue tracker. The SDK team will respond to all newly filed issues within two business days. For issues or requests that are more generally related to the LaunchDarkly Go SDK, rather than specifically for the code in this repository, please use the [`go-server-sdk`](https://github.com/launchdarkly/go-server-sdk) repository.
 
## Submitting pull requests
 
We encourage pull requests and other contributions from the community. Before submitting pull requests, ensure that all temporary or unintended code is removed. Don't worry about adding reviewers to the pull request; the LaunchDarkly SDK team will add themselves. The SDK team will acknowledge all pull requests within two business days.
 
## Build instructions
 
### Prerequisites
 
This project should be built against the lowest supported Go version as described in [README.md](./README.md).

### Building

To build the project without running any tests:
```
make
```

If you wish to clean your working directory between builds, you can clean it by running:
```
make clean
```

To run the linter:
```
make lint
```

### Testing
 
To build and run all unit tests:
```
make test
```

## Coding best practices

### Test coverage

It is important to keep unit test coverage as close to 100% as possible in this project. You can generate this information locally with `make test-coverage`.

The build will fail if there are any uncovered blocks of code, unless you explicitly add an override by placing a comment that starts with `// COVERAGE` somewhere within that block. Sometimes a gap in coverage is unavoidable, usually because the compiler requires us to provide a code path for some condition that in practice can't happen and can't be tested. Exclude these paths with a `// COVERAGE` comment.

Many of the parameterized tests in this project are redundant with contract test cases in `sdk-test-harness`. This is deliberate. The contract tests cover a thorough set of permutations of evaluation inputs for the server-side SDKs in general, including the Go SDK-- but, since the `go-server-sdk-evaluation` code is also used outside of the SDK, it's desirable to have equally thorough test coverage within this project.

### Avoid heap allocations

The Go SDK can be used in high-traffic application/service code where performance is critical. There are a number of coding principles to keep in mind for maximizing performance. The benchmarks that are run in CI are helpful in measuring the impact of code changes in this regard.

Go's memory model uses a mix of stack and heap allocations, with the compiler transparently choosing the most appropriate strategy based on various type and scope rules. It is always preferable, when possible, to keep ephemeral values on the stack rather than on the heap to avoid creating extra work for the garbage collector.

- The most obvious rule is that anything explicitly allocated by reference (`x := &SomeType{}`), or returned by reference (`return &x`), will be allocated on the heap. Avoid this unless the object has mutable state that must be shared.
- Casting a value type to an interface causes it to be allocated on the heap, since an interface is really a combination of a type identifier and a hidden pointer. The exception is if the value is a renamed simple type such as a string.
- A closure that references any variables outside of its scope (including the method receiver, if it is inside a method) causes an object to be allocated on the heap containing the values or addresses of those variables.
- Treating a method as an anonymous function (`myFunc := someReceiver.SomeMethod`) is equivalent to a closure.

Allocations are counted in the benchmark output: "5 allocs/op" means that a total of 5 heap objects were allocated during each run of the benchmark. This does not mean that the objects were retained, only that they were allocated at some point.

For methods that should be guaranteed _not_ to do any heap allocations, the corresponding benchmarks should have names ending in `NoAlloc`. The `make benchmarks` target will automatically fail if allocations are detected in any benchmarks that have this name suffix.

For a much (MUCH) more detailed breakdown of this behavior, you may use the option `GODEBUG=allocfreetrace=1` while running a unit test or benchmark. This provides the type and code location of literally every heap allocation during the run. The output is extremely verbose, so it is recommended that you:

1. use the Makefile helper `benchmark-allocs` (see below) to reduce the number of benchmark runs and avoid capturing allocations from the Go tools themselves;
2. search the stacktrace output to find the method you are actually testing (such as `BoolVariation`) rather than the benchmark function name, so you are not looking at actions that are just part of the benchmark setup;
3. consider writing a smaller temporary benchmark specifically for this purpose, since most of the existing benchmarks will iterate over a series of parameters.

```bash
BENCHMARK=BenchmarkMySampleOperation make benchmark-allocs
```

#### Cases where heap allocations are unavoidable

The following scenarios necessarily involve some heap allocations, even though we would still like to minimize these.

1. We query a prerequisite flag, or a segment, and the data provider (based on the SDK configuration) decides to do a database query instead of just getting data from memory. That's outside of the evaluation engine's control. All of the tests and benchmarks defined in this package use a simple in-memory data provider.
2. We need to query big segments data. Big segments always involve a database.
3. Recursive evaluations of prerequisites or segments exceed the preallocated stack depth (see the `preallocated`
constants defined in `evaluator.go`).
