package fdv2proto

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

func ToStorableItems(events []Event) []ldstoretypes.Collection {
	flagCollection := ldstoretypes.Collection{
		Kind:  datakinds.Features,
		Items: make([]ldstoretypes.KeyedItemDescriptor, 0),
	}

	segmentCollection := ldstoretypes.Collection{
		Kind:  datakinds.Segments,
		Items: make([]ldstoretypes.KeyedItemDescriptor, 0),
	}

	for _, event := range events {
		switch e := event.(type) {
		case PutObject:
			switch e.Kind {
			case FlagKind:
				flagCollection.Items = append(flagCollection.Items, ldstoretypes.KeyedItemDescriptor{
					Key:  e.Key,
					Item: e.Object,
				})
			case SegmentKind:
				segmentCollection.Items = append(segmentCollection.Items, ldstoretypes.KeyedItemDescriptor{
					Key:  e.Key,
					Item: e.Object,
				})
			}
		case DeleteObject:
			switch e.Kind {
			case FlagKind:
				flagCollection.Items = append(flagCollection.Items, ldstoretypes.KeyedItemDescriptor{
					Key: e.Key,
					Item: ldstoretypes.ItemDescriptor{
						Version: e.Version,
						Item:    nil,
					},
				})
			case SegmentKind:
				segmentCollection.Items = append(segmentCollection.Items, ldstoretypes.KeyedItemDescriptor{
					Key: e.Key,
					Item: ldstoretypes.ItemDescriptor{
						Version: e.Version,
						Item:    nil,
					},
				})
			}
		}
	}

	return []ldstoretypes.Collection{flagCollection, segmentCollection}
}
