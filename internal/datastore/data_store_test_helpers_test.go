package datastore

import (
	"errors"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type unknownDataKind struct{}

func (k unknownDataKind) GetName() string {
	return "unknown"
}

func (k unknownDataKind) Serialize(item interfaces.StoreItemDescriptor) []byte {
	return nil
}

func (k unknownDataKind) Deserialize(data []byte) (interfaces.StoreItemDescriptor, error) {
	return interfaces.StoreItemDescriptor{}, errors.New("not implemented")
}
