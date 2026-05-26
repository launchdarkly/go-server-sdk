module github.com/launchdarkly/go-server-sdk/ldai

go 1.24.0

require (
	github.com/alexkappa/mustache v1.0.0
	github.com/google/uuid v1.1.1
	github.com/launchdarkly/go-sdk-common/v4 v4.0.0-20260526225240-97f2812dbb86
	github.com/launchdarkly/go-server-sdk/v7 v7.7.0
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/launchdarkly/go-jsonstream/v4 v4.0.0-20260526224546-8bf6dec4a0c8 // indirect
	github.com/launchdarkly/go-sdk-events/v3 v3.6.1-0.20260526230019-c1af04865d66 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// During v4 development, point at the parent's v4-bumped sources for ldai's own tests.
// Removed before tagging v4.0.0 — see Stage 5 of the SDK-2113 cascade plan.
replace github.com/launchdarkly/go-server-sdk/v7 => ../
