module github.com/launchdarkly/go-server-sdk/v6/testservice

go 1.16

require (
	github.com/gorilla/mux v1.8.0
	github.com/launchdarkly/go-sdk-common/v3 v3.0.0-alpha.pub.3
	github.com/launchdarkly/go-server-sdk/v6 v6.0.0
)

replace github.com/launchdarkly/go-server-sdk/v6 => ../
