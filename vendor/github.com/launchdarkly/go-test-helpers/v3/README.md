# LaunchDarkly Go Test Helpers

[![Circle CI](https://circleci.com/gh/launchdarkly/go-test-helpers.svg?style=svg)](https://circleci.com/gh/launchdarkly/go-test-helpers) [![Documentation](https://img.shields.io/static/v1?label=go.dev&message=reference&color=00add8)](https://pkg.go.dev/github.com/launchdarkly/go-test-helpers)

This project centralizes some test support code that is used by LaunchDarkly's Go SDK and related components, and that may be useful in other Go projects.

While this code may be useful in other projects, it is primarily geared toward LaunchDarkly's own development needs and is not meant to provide a large general-purpose framework. It is meant for unit test code and should not be used as a runtime dependency.

This version of the project requires Go 1.18 or higher.

## Contents

The main package provides general-purpose helper functions.

Subpackage `httphelpers` provides convenience wrappers for using `net/http` and `net/http/httptest` in test code.

Subpackage `jsonhelpers` provides functions for manipulating JSON.

Subpackage `matchers` contains a test assertion API with combinators.

Subpackage `testbox` provides the ability to write tests-of-tests within the Go testing framework.

## Usage

Import any of these packages in your test code:

```go
import (
    "github.com/launchdarkly/go-test-helpers/v3"
    "github.com/launchdarkly/go-test-helpers/v3/httphelpers"
    "github.com/launchdarkly/go-test-helpers/v3/jsonhelpers"
    "github.com/launchdarkly/go-test-helpers/v3/ldservices"
    "github.com/launchdarkly/go-test-helpers/v3/testbox"
)
```

Breaking changes will only be made in a new major version. It is advisable to use a dependency manager to pin these dependencies to a module version or a major version branch.

## Contributing

We encourage pull requests and other contributions from the community. Check out our [contributing guidelines](CONTRIBUTING.md) for instructions on how to contribute to this project.

## About LaunchDarkly

* LaunchDarkly is a continuous delivery platform that provides feature flags as a service and allows developers to iterate quickly and safely. We allow you to easily flag your features and manage them from the LaunchDarkly dashboard.  With LaunchDarkly, you can:
    * Roll out a new feature to a subset of your users (like a group of users who opt-in to a beta tester group), gathering feedback and bug reports from real-world use cases.
    * Gradually roll out a feature to an increasing percentage of users, and track the effect that the feature has on key metrics (for instance, how likely is a user to complete a purchase if they have feature A versus feature B?).
    * Turn off a feature that you realize is causing performance problems in production, without needing to re-deploy, or even restart the application with a changed configuration file.
    * Grant access to certain features based on user attributes, like payment plan (eg: users on the ‘gold’ plan get access to more features than users in the ‘silver’ plan). Disable parts of your application to facilitate maintenance, without taking everything offline.
* LaunchDarkly provides feature flag SDKs for a wide variety of languages and technologies. Check out [our documentation](https://docs.launchdarkly.com/docs) for a complete list.
* Explore LaunchDarkly
    * [launchdarkly.com](https://www.launchdarkly.com/ "LaunchDarkly Main Website") for more information
    * [docs.launchdarkly.com](https://docs.launchdarkly.com/  "LaunchDarkly Documentation") for our documentation and SDK reference guides
    * [apidocs.launchdarkly.com](https://apidocs.launchdarkly.com/  "LaunchDarkly API Documentation") for our API documentation
    * [blog.launchdarkly.com](https://blog.launchdarkly.com/  "LaunchDarkly Blog Documentation") for the latest product updates
    * [Feature Flagging Guide](https://github.com/launchdarkly/featureflags/  "Feature Flagging Guide") for best practices and strategies
