//go:build launchdarkly_easyjson
// +build launchdarkly_easyjson

package datasourcev2

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// This file contains the payload decoders used by the launchdarkly_easyjson build. It retains
// the reflection-based decode (encoding/json over the polling envelope and each put-object
// event) so that nothing in this build depends on jreader.RawValue: easyjson support is planned
// for removal from go-jsonstream, and no new code should rely on its token reader.
//
// The results are identical to the default build's single-pass decoder -- including the eager
// item deserialization that lets the ChangeSet pre-populate its collections -- at the cost of
// the extra reflection scans and byte copies that the default build avoids.

// parsePollingPayload decodes a polling response body of the form {"events":[...]} and returns
// the completed change set. Refer to the default build's implementation for the event semantics;
// the two are behaviorally equivalent.
func parsePollingPayload(ctx context.Context, body []byte) (*subsystems.ChangeSet, error) {
	var payload subsystems.PollingPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	builder := subsystems.NewChangeSetBuilder()
	for _, event := range payload.Events {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		switch event.Name {
		case subsystems.EventServerIntent:
			intent, err := parseServerIntentEventData(event.Data)
			if err != nil {
				return nil, err
			}
			if intent.Payload.Code == subsystems.IntentNone {
				return builder.NoChanges(), nil
			}
			builder.Start(intent)
		case subsystems.EventPutObject:
			put, err := parsePutObjectEventData(event.Data)
			if err != nil {
				return nil, err
			}
			put.addTo(builder)
		case subsystems.EventDeleteObject:
			deleteObject, err := parseDeleteObjectEventData(event.Data)
			if err != nil {
				return nil, err
			}
			builder.AddDelete(deleteObject.Kind, deleteObject.Key, deleteObject.Version)
		case subsystems.EventPayloadTransferred:
			selector, err := parseSelectorEventData(event.Data)
			if err != nil {
				return nil, err
			}
			return builder.Finish(selector)
		default:
			// An unknown event name is ignored for forwards compatibility.
		}
	}
	return nil, errors.New(errNoKnownPollingEvents)
}

// parsePutObjectEventData decodes the data of a put-object event via encoding/json, then eagerly
// deserializes the item so the change set can carry the parsed form alongside the raw bytes.
func parsePutObjectEventData(data []byte) (parsedPutObject, error) {
	var put subsystems.PutObject
	if err := json.Unmarshal(data, &put); err != nil {
		return parsedPutObject{}, err
	}
	p := parsedPutObject{kind: put.Kind, key: put.Key, version: put.Version, object: put.Object}
	kind, recognized := put.Kind.ToFDV1()
	if !recognized || put.Object == nil {
		// An unrecognized kind is kept raw for forwards compatibility; its JSON syntax was
		// already validated by the json.RawMessage decode above.
		return p, nil
	}
	itemReader := jreader.NewReader(put.Object)
	item, err := kind.DeserializeFromJSONReader(&itemReader)
	if err != nil {
		return p, err
	}
	p.item, p.hasItem = item, true
	return p, nil
}
