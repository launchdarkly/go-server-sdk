package datastore

import (
	"errors"

	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"
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
