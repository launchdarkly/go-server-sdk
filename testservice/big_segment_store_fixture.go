package main

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	cf "github.com/launchdarkly/go-server-sdk/v7/testservice/servicedef/callbackfixtures"
)

type BigSegmentStoreFixture struct {
	service *callbackService
}

type bigSegmentMembershipMap map[string]bool

func (b *BigSegmentStoreFixture) Build(context subsystems.ClientContext) (subsystems.BigSegmentStore, error) {
	return b, nil
}

func (b *BigSegmentStoreFixture) Close() error {
	return b.service.close()
}

func (b *BigSegmentStoreFixture) GetMetadata() (subsystems.BigSegmentStoreMetadata, error) {
	var resp cf.BigSegmentStoreGetMetadataResponse
	if err := b.service.post(cf.BigSegmentStorePathGetMetadata, nil, &resp); err != nil {
		return subsystems.BigSegmentStoreMetadata{}, err
	}
	return subsystems.BigSegmentStoreMetadata(resp), nil
}

func (b *BigSegmentStoreFixture) GetMembership(contextHash string) (subsystems.BigSegmentMembership, error) {
	params := cf.BigSegmentStoreGetMembershipParams{ContextHash: contextHash, UserHash: contextHash}
	var resp cf.BigSegmentStoreGetMembershipResponse
	if err := b.service.post(cf.BigSegmentStorePathGetMembership, params, &resp); err != nil {
		return nil, err
	}
	return bigSegmentMembershipMap(resp.Values), nil
}

func (m bigSegmentMembershipMap) CheckMembership(segmentRef string) ldvalue.OptionalBool {
	if value, ok := m[segmentRef]; ok {
		return ldvalue.NewOptionalBool(value)
	}
	return ldvalue.OptionalBool{}
}
