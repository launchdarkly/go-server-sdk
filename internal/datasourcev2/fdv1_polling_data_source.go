package datasourcev2

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type fdv1ToFDv2Requester struct {
	requester *datasource.PollingRequester
	loggers   ldlog.Loggers
}

func (r *fdv1ToFDv2Requester) Request(
	ctx context.Context,
	selector subsystems.Selector,
) (*subsystems.ChangeSet, http.Header, error) {
	data, cached, headers, err := r.requester.Request()
	if err != nil {
		return nil, headers, err
	}

	code := subsystems.IntentTransferFull
	reason := "cant-catchup"

	if cached {
		code = subsystems.IntentNone
		reason = "cached"
	}

	changeSetBuilder := subsystems.NewChangeSetBuilder()
	changeSetBuilder.Start(subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "arbitrary-id",
			Target: 0,
			Code:   code,
			Reason: reason,
		},
	})

	for _, item := range data {
		kind := subsystems.FlagKind
		if item.Kind == datakinds.Segments {
			kind = subsystems.SegmentKind
		}

		for _, keyedItem := range item.Items {
			if keyedItem.Item.Item == nil {
				continue
			}

			bytes, err := json.Marshal(keyedItem.Item.Item)
			if err != nil {
				r.loggers.Warn("Error marshalling v1 item to JSON: %s", err)
				return nil, headers, err
			}

			changeSetBuilder.AddPut(kind, keyedItem.Key, keyedItem.Item.Version, bytes)
		}
	}

	changeset, err := changeSetBuilder.Finish(subsystems.NewSelector("state", 1))
	return changeset, headers, err
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
	cfg datasource.PollingConfig,
) *PollingProcessor {
	requester := &fdv1ToFDv2Requester{
		requester: datasource.NewPollingRequester(context, context.GetHTTP().CreateHTTPClient(), cfg.BaseURI, cfg.FilterKey),
		loggers:   context.GetLogging().Loggers,
	}
	return newPollingProcessor(context, requester, cfg.PollInterval)
}
