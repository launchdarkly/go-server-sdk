package ldclient

import "github.com/launchdarkly/go-sdk-common/v3/ldvalue"

// lazyEnvironmentIDProvider reads the environment ID from the client's data system at call time.
// The hook runner is created before the data system exists, so it cannot hold the data system's
// provider directly.
type lazyEnvironmentIDProvider struct {
	client *LDClient
}

func (p lazyEnvironmentIDProvider) GetEnvironmentID() ldvalue.OptionalString {
	if p.client.dataSystem == nil {
		return ldvalue.OptionalString{}
	}
	return p.client.dataSystem.EnvironmentIDProvider().GetEnvironmentID()
}
