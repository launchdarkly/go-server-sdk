module github.com/launchdarkly/go-server-sdk/v6/testservice

go 1.18

require (
	github.com/gorilla/mux v1.8.0
	github.com/launchdarkly/go-sdk-common/v3 v3.0.0-alpha.pub.13
	github.com/launchdarkly/go-server-sdk/v6 v6.0.0
	github.com/launchdarkly/go-test-helpers/v2 v2.3.1 // indirect
)

replace github.com/launchdarkly/go-server-sdk/v6 => ../
