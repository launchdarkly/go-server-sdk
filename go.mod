module github.com/launchdarkly/go-server-sdk/v7

go 1.24.0

require (
	github.com/fsnotify/fsnotify v1.4.7
	github.com/google/uuid v1.1.1
	github.com/gregjones/httpcache v0.0.0-20171119193500-2bcd89a1743f
	github.com/launchdarkly/ccache v1.1.0
	github.com/launchdarkly/eventsource v1.10.0
	github.com/launchdarkly/go-jsonstream/v3 v3.1.1
	github.com/launchdarkly/go-ntlm-proxy-auth v1.0.3
	github.com/launchdarkly/go-sdk-common/v3 v3.5.0
	github.com/launchdarkly/go-sdk-events/v3 v3.6.2-0.20260610185926-04050b02df99
	github.com/launchdarkly/go-server-sdk-evaluation/v3 v3.0.1
	github.com/launchdarkly/go-test-helpers/v3 v3.1.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.8.0
	gopkg.in/ghodss/yaml.v1 v1.0.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/launchdarkly/go-ntlmssp v1.0.3 // indirect
	github.com/launchdarkly/go-semver v1.0.3 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// v7.15.1 and v7.15.2 upgraded to the go-jsonstream/v4, go-sdk-common/v4, and
// go-server-sdk-evaluation/v4 core libraries. Those /v4 major bumps are a breaking
// change for customers (Go semantic import versioning), so these releases are
// retracted in favor of a v3-only release. See SDK-2496.
retract (
	v7.15.1
	v7.15.2
)
