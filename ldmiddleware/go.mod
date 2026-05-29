module github.com/launchdarkly/go-server-sdk/ldmiddleware

go 1.24.0

require (
	github.com/felixge/httpsnoop v1.0.4
	github.com/google/uuid v1.1.1
	github.com/launchdarkly/go-sdk-common/v4 v4.0.0
	github.com/launchdarkly/go-server-sdk/v7 v7.13.4
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20171119193500-2bcd89a1743f // indirect
	github.com/launchdarkly/ccache v1.1.0 // indirect
	github.com/launchdarkly/eventsource v1.10.0 // indirect
	github.com/launchdarkly/go-jsonstream/v4 v4.0.0 // indirect
	github.com/launchdarkly/go-sdk-events/v3 v3.6.1 // indirect
	github.com/launchdarkly/go-semver v1.0.3 // indirect
	github.com/launchdarkly/go-server-sdk-evaluation/v4 v4.0.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/launchdarkly/go-server-sdk/v7 => ../
