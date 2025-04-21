package datasourcev2

import (
	"context"
	"encoding/json"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type fdv1ToFDv2Requester struct {
	requester *datasource.PollingRequester
	loggers   ldlog.Loggers
}

func (r *fdv1ToFDv2Requester) Request(ctx context.Context, selector fdv2proto.Selector) (*fdv2proto.ChangeSet, error) {
	data, cached, err := r.requester.Request()
	if err != nil {
		return nil, err
	}

	code := fdv2proto.IntentTransferFull
	reason := "cant-catchup"

	if cached {
		code = fdv2proto.IntentNone
		reason = "cached"
	}

	changeSetBuilder := fdv2proto.NewChangeSetBuilder()
	err = changeSetBuilder.Start(fdv2proto.ServerIntent{
		Payload: fdv2proto.Payload{
			ID:     "arbitrary-id",
			Target: 0,
			Code:   code,
			Reason: reason,
		},
	})
	if err != nil {
		return nil, err
	}

	for _, item := range data {
		kind := fdv2proto.FlagKind
		if item.Kind == datakinds.Segments {
			kind = fdv2proto.SegmentKind
		}

		for _, keyedItem := range item.Items {
			if keyedItem.Item.Item == nil {
				continue
			}

			bytes, err := json.Marshal(keyedItem.Item.Item)
			if err != nil {
				r.loggers.Warn("Error marshalling v1 item to JSON: %s", err)
			}

			changeSetBuilder.AddPut(kind, keyedItem.Key, keyedItem.Item.Version, bytes)
		}
	}

	return changeSetBuilder.Finish(fdv2proto.NewSelector("state", 1))
}

func (r *fdv1ToFDv2Requester) BaseURI() string {
	return r.requester.BaseURI()
}

func (r *fdv1ToFDv2Requester) FilterKey() string {
	return r.requester.FilterKey()
}

// NewFDv1PollingProcessor creates the internal implementation of the polling data source.
func NewFDv1PollingProcessor(
	context subsystems.ClientContext,
	dataDestination subsystems.DataDestination,
	cfg datasource.PollingConfig,
) *PollingProcessor {
	requester := &fdv1ToFDv2Requester{
		requester: datasource.NewPollingRequester(context, context.GetHTTP().CreateHTTPClient(), cfg.BaseURI, cfg.FilterKey),
		loggers:   context.GetLogging().Loggers,
	}
	return newPollingProcessor(context, dataDestination, requester, cfg.PollInterval)
}
