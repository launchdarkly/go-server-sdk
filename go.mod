module github.com/launchdarkly/go-server-sdk/v6

go 1.18

require (
	github.com/fsnotify/fsnotify v1.4.7
	github.com/gregjones/httpcache v0.0.0-20171119193500-2bcd89a1743f
	github.com/launchdarkly/ccache v1.1.0
	github.com/launchdarkly/eventsource v1.6.2
	github.com/launchdarkly/go-jsonstream/v3 v3.0.0
	github.com/launchdarkly/go-ntlm-proxy-auth v1.0.1
	github.com/launchdarkly/go-sdk-common/v3 v3.0.1
	github.com/launchdarkly/go-sdk-events/v2 v2.0.2
	github.com/launchdarkly/go-server-sdk-evaluation/v2 v2.0.2
	github.com/launchdarkly/go-test-helpers/v3 v3.0.2
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/stretchr/testify v1.7.0
	golang.org/x/exp v0.0.0-20220823124025-807a23277127
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gopkg.in/ghodss/yaml.v1 v1.0.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/launchdarkly/go-ntlmssp v1.0.1 // indirect
	github.com/launchdarkly/go-semver v1.0.2 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	golang.org/x/crypto v0.1.0 // indirect
	golang.org/x/sys v0.1.0 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)

replace github.com/launchdarkly/go-sdk-common/v3 => github.com/launchdarkly/go-sdk-common-private/v3 v3.0.0-alpha.6.0.20230818171508-4d48cc1dba90

replace github.com/launchdarkly/go-sdk-events/v2 => github.com/launchdarkly/go-sdk-events-private/v2 v2.0.0-alpha.5.0.20230828175712-cf4fc6d12b81

replace github.com/launchdarkly/go-server-sdk-evaluation/v2 => github.com/launchdarkly/go-server-sdk-evaluation-private/v2 v2.0.0-alpha.7.0.20230828162739-3458fcf551b1
