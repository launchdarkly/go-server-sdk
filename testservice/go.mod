module github.com/launchdarkly/go-server-sdk/v7/testservice

go 1.18

require (
	github.com/gorilla/mux v1.8.0
	github.com/launchdarkly/go-sdk-common/v3 v3.2.0
	github.com/launchdarkly/go-server-sdk/v7 v7.0.0
)

require (
	github.com/google/uuid v1.1.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20171119193500-2bcd89a1743f // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/launchdarkly/ccache v1.1.0 // indirect
	github.com/launchdarkly/eventsource v1.6.2 // indirect
	github.com/launchdarkly/go-jsonstream/v3 v3.1.0 // indirect
	github.com/launchdarkly/go-sdk-events/v3 v3.4.0 // indirect
	github.com/launchdarkly/go-semver v1.0.3 // indirect
	github.com/launchdarkly/go-server-sdk-evaluation/v3 v3.0.1 // indirect
	github.com/launchdarkly/go-test-helpers/v2 v2.3.1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	golang.org/x/exp v0.0.0-20240808152545-0cdaa3abc0fa // indirect
	golang.org/x/sync v0.8.0 // indirect
)

replace github.com/launchdarkly/go-server-sdk/v7 => ../
