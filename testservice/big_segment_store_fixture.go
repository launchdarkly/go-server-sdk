package main

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	cf "gopkg.in/launchdarkly/go-server-sdk.v6/testservice/servicedef/callbackfixtures"
)

type BigSegmentStoreFixture struct {
	service *callbackService
}

type bigSegmentMembershipMap map[string]bool

func (b *BigSegmentStoreFixture) CreateBigSegmentStore(context interfaces.ClientContext) (interfaces.BigSegmentStore, error) {
	return b, nil
}

func (b *BigSegmentStoreFixture) Close() error {
	return b.service.close()
}

func (b *BigSegmentStoreFixture) GetMetadata() (interfaces.BigSegmentStoreMetadata, error) {
	var resp cf.BigSegmentStoreGetMetadataResponse
	if err := b.service.post(cf.BigSegmentStorePathGetMetadata, nil, &resp); err != nil {
		return interfaces.BigSegmentStoreMetadata{}, err
	}
	return interfaces.BigSegmentStoreMetadata(resp), nil
}

func (b *BigSegmentStoreFixture) GetUserMembership(userHash string) (interfaces.BigSegmentMembership, error) {
	params := cf.BigSegmentStoreGetMembershipParams{UserHash: userHash}
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
