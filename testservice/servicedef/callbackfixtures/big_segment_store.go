package callbackfixtures

import "gopkg.in/launchdarkly/go-sdk-common.v3/ldtime"

const (
	BigSegmentStorePathGetMetadata   = "/getMetadata"
	BigSegmentStorePathGetMembership = "/getMembership"
)

type BigSegmentStoreGetMetadataResponse struct {
	LastUpToDate ldtime.UnixMillisecondTime `json:"lastUpToDate"`
}

type BigSegmentStoreGetMembershipParams struct {
	ContextHash string `json:"contextHash"`
	UserHash    string `json:"userHash"` // temporary, for compatibility with current sdk-test-harness
}

type BigSegmentStoreGetMembershipResponse struct {
	Values map[string]bool `json:"values,omitempty"`
}
