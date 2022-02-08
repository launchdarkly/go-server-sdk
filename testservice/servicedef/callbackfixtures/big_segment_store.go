package callbackfixtures

import "gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

const (
	BigSegmentStorePathGetMetadata   = "/getMetadata"
	BigSegmentStorePathGetMembership = "/getMembership"
)

type BigSegmentStoreGetMetadataResponse struct {
	LastUpToDate ldtime.UnixMillisecondTime `json:"lastUpToDate"`
}

type BigSegmentStoreGetMembershipParams struct {
	UserHash string `json:"userHash"`
}

type BigSegmentStoreGetMembershipResponse struct {
	Values map[string]bool `json:"values,omitempty"`
}
