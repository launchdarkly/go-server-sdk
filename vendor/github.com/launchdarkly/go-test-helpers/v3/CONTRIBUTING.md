# Contributing to go-test-helpers
 
## Submitting bug reports and feature requests

The LaunchDarkly SDK team maintains this repository and monitors the [issue tracker](https://github.com/launchdarkly/go-test-helpers/issues) there. Bug reports and feature requests specific to this project should be filed in this issue tracker. The team will respond to all newly filed issues within two business days.
 
## Submitting pull requests
 
We encourage pull requests and other contributions from the community. Before submitting pull requests, ensure that all temporary or unintended code is removed. Don't worry about adding reviewers to the pull request; the LaunchDarkly SDK team will add themselves. The SDK team will acknowledge all pull requests within two business days.
 
## Build instructions
 
### Prerequisites
 
This project should be built against Go 1.13 or newer.

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
 
To build the project and run all unit tests:
```
make test
```
