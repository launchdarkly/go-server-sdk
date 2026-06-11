module github.com/launchdarkly/go-server-sdk/ldai

go 1.24.0

require (
	github.com/alexkappa/mustache v1.0.0
	github.com/google/uuid v1.1.1
	github.com/launchdarkly/go-sdk-common/v3 v3.5.0
	github.com/launchdarkly/go-server-sdk/v7 v7.15.0
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/launchdarkly/go-jsonstream/v3 v3.1.1 // indirect
	github.com/launchdarkly/go-sdk-events/v3 v3.6.2 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v0.9.1 // Introduced unintentional breaking changes; use version v0.9.2 or later.
