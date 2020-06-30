package datastore

import (
	"errors"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
)

type unknownDataKind struct{}

func (k unknownDataKind) GetName() string {
	return "unknown"
}

func (k unknownDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	return nil
}

func (k unknownDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{}, errors.New("not implemented")
}
