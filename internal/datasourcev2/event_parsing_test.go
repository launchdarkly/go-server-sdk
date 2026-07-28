package datasourcev2

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestFlagJSON(t testing.TB, key string, version int) []byte {
	t.Helper()
	flag := ldbuilders.NewFlagBuilder(key).Version(version).
		On(true).
		Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		FallthroughVariation(0).
		AddTarget(0, "user-a", "user-b").
		AddRule(ldbuilders.NewRuleBuilder().ID("rule-1").Variation(1).Clauses(
			ldbuilders.Clause("email", "in", ldvalue.String("test@example.com")),
		)).
		Build()
	return datakinds.Features.Serialize(ldstoretypes.ItemDescriptor{Version: version, Item: &flag})
}

func makeTestSegmentJSON(t testing.TB, key string, version int) []byte {
	t.Helper()
	segment := ldbuilders.NewSegmentBuilder(key).Version(version).Included("user-a").Build()
	return datakinds.Segments.Serialize(ldstoretypes.ItemDescriptor{Version: version, Item: &segment})
}

func TestParsePutObjectEventDataPropertyOrderings(t *testing.T) {
	flagJSON := makeTestFlagJSON(t, "flagkey", 3)

	kindFirst := fmt.Sprintf(`{"kind":"flag","key":"flagkey","version":3,"object":%s}`, flagJSON)
	objectFirst := fmt.Sprintf(`{"object":%s,"version":3,"key":"flagkey","kind":"flag"}`, flagJSON)

	for name, data := range map[string]string{"kind before object": kindFirst, "object before kind": objectFirst} {
		t.Run(name, func(t *testing.T) {
			p, err := parsePutObjectEventData([]byte(data))
			require.NoError(t, err)
			assert.Equal(t, subsystems.FlagKind, p.kind)
			assert.Equal(t, "flagkey", p.key)
			assert.Equal(t, 3, p.version)
			assert.JSONEq(t, string(flagJSON), string(p.object))
			require.True(t, p.hasItem)
			assert.Equal(t, 3, p.item.Version)
			require.IsType(t, &ldmodel.FeatureFlag{}, p.item.Item)
			assert.Equal(t, "flagkey", p.item.Item.(*ldmodel.FeatureFlag).Key)
		})
	}
}

func TestParsePutObjectEventDataSegmentKind(t *testing.T) {
	segmentJSON := makeTestSegmentJSON(t, "segmentkey", 2)
	data := fmt.Sprintf(`{"kind":"segment","key":"segmentkey","version":2,"object":%s}`, segmentJSON)
	p, err := parsePutObjectEventData([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, subsystems.SegmentKind, p.kind)
	require.True(t, p.hasItem)
	require.IsType(t, &ldmodel.Segment{}, p.item.Item)
	assert.Equal(t, "segmentkey", p.item.Item.(*ldmodel.Segment).Key)
}

func TestParsePutObjectEventDataUnknownKind(t *testing.T) {
	t.Run("valid object is retained raw", func(t *testing.T) {
		data := `{"kind":"future-kind","key":"k","version":1,"object":{"key":"k","futureProp":[1,2]}}`
		p, err := parsePutObjectEventData([]byte(data))
		require.NoError(t, err)
		assert.Equal(t, subsystems.ObjectKind("future-kind"), p.kind)
		assert.False(t, p.hasItem)
		assert.JSONEq(t, `{"key":"k","futureProp":[1,2]}`, string(p.object))
	})

	t.Run("malformed object is still an error", func(t *testing.T) {
		// The braces balance, so this proves the object bytes are fully validated even though
		// an unrecognized kind is never model-parsed (in the default build that validation
		// comes from jreader.RawValue itself; in the easyjson build, from the json.RawMessage
		// decode).
		data := `{"kind":"future-kind","key":"k","version":1,"object":{"bad":}}`
		_, err := parsePutObjectEventData([]byte(data))
		assert.Error(t, err)
	})
}

func TestParsePutObjectEventDataMalformedObjectOfKnownKind(t *testing.T) {
	data := `{"kind":"flag","key":"k","version":1,"object":{"key":12345,"on":"not-a-bool"}}`
	_, err := parsePutObjectEventData([]byte(data))
	assert.Error(t, err)
}

func TestParsePutObjectEventDataPreservesObjectBytesExactly(t *testing.T) {
	// Whitespace surrounding the object value must not leak into the captured raw bytes, and
	// whitespace inside the value must be preserved verbatim -- relay embeds these bytes
	// unmodified into downstream events.
	data := `{ "kind": "flag", "key": "flagkey", "version": 1,` + "\n\t" +
		`"object": { "key": "flagkey", "version": 1, "on": true } }`
	p, err := parsePutObjectEventData([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, `{ "key": "flagkey", "version": 1, "on": true }`, string(p.object))
	require.True(t, p.hasItem)
	assert.Equal(t, "flagkey", p.item.Item.(*ldmodel.FeatureFlag).Key)
}

func TestParsePutObjectEventDataIgnoresExtraProperties(t *testing.T) {
	flagJSON := makeTestFlagJSON(t, "flagkey", 1)
	data := fmt.Sprintf(`{"kind":"flag","futureField":{"a":[1]},"key":"flagkey","version":1,"object":%s}`, flagJSON)
	p, err := parsePutObjectEventData([]byte(data))
	require.NoError(t, err)
	assert.True(t, p.hasItem)
}

func TestParseServerIntentEventData(t *testing.T) {
	t.Run("single payload", func(t *testing.T) {
		data := `{"payloads":[{"id":"p1","target":3,"intentCode":"xfer-full","reason":"payload-missing"}]}`
		intent, err := parseServerIntentEventData([]byte(data))
		require.NoError(t, err)
		assert.Equal(t, "p1", intent.Payload.ID)
		assert.Equal(t, 3, intent.Payload.Target)
		assert.Equal(t, subsystems.IntentTransferFull, intent.Payload.Code)
		assert.Equal(t, "payload-missing", intent.Payload.Reason)
	})

	t.Run("only the first of multiple payloads is used", func(t *testing.T) {
		data := `{"payloads":[{"id":"p1","intentCode":"none"},{"id":"p2","intentCode":"xfer-full"}]}`
		intent, err := parseServerIntentEventData([]byte(data))
		require.NoError(t, err)
		assert.Equal(t, "p1", intent.Payload.ID)
		assert.Equal(t, subsystems.IntentNone, intent.Payload.Code)
	})

	t.Run("empty payloads is an error", func(t *testing.T) {
		_, err := parseServerIntentEventData([]byte(`{"payloads":[]}`))
		assert.Error(t, err)
	})

	t.Run("missing payloads is an error", func(t *testing.T) {
		_, err := parseServerIntentEventData([]byte(`{}`))
		assert.Error(t, err)
	})
}

func TestParseDeleteObjectEventData(t *testing.T) {
	data := `{"version":7,"kind":"segment","key":"gone"}`
	d, err := parseDeleteObjectEventData([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, subsystems.DeleteObject{Version: 7, Kind: subsystems.SegmentKind, Key: "gone"}, d)
}

func TestParseSelectorEventData(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		selector, err := parseSelectorEventData([]byte(`{"state":"s1","version":9}`))
		require.NoError(t, err)
		assert.Equal(t, "s1", selector.State())
		assert.Equal(t, 9, selector.Version())
	})

	t.Run("missing state is an error", func(t *testing.T) {
		_, err := parseSelectorEventData([]byte(`{"version":9}`))
		assert.Error(t, err)
	})

	t.Run("missing version is an error", func(t *testing.T) {
		_, err := parseSelectorEventData([]byte(`{"state":"s1"}`))
		assert.Error(t, err)
	})
}

func TestParseGoodbyeEventData(t *testing.T) {
	goodbye, err := parseGoodbyeEventData([]byte(`{"reason":"see ya","silent":true}`))
	require.NoError(t, err)
	assert.Equal(t, "see ya", goodbye.Reason)
}

func TestParseErrorEventData(t *testing.T) {
	errorData, err := parseErrorEventData([]byte(`{"payloadId":"p1","reason":"broke"}`))
	require.NoError(t, err)
	assert.Equal(t, subsystems.Error{PayloadID: "p1", Reason: "broke"}, errorData)
}

func makePollingBody(t testing.TB, flagCount int) []byte {
	t.Helper()
	var sb strings.Builder
	sb.WriteString(`{"events":[`)
	sb.WriteString(`{"event":"server-intent","data":{"payloads":[{"id":"p1","target":1,"intentCode":"xfer-full","reason":"payload-missing"}]}}`)
	for i := 0; i < flagCount; i++ {
		fmt.Fprintf(&sb, `,{"event":"put-object","data":{"kind":"flag","key":"flag-%d","version":%d,"object":%s}}`,
			i, i+1, makeTestFlagJSON(t, fmt.Sprintf("flag-%d", i), i+1))
	}
	fmt.Fprintf(&sb, `,{"event":"put-object","data":{"kind":"segment","key":"segment-0","version":1,"object":%s}}`,
		makeTestSegmentJSON(t, "segment-0", 1))
	sb.WriteString(`,{"event":"delete-object","data":{"kind":"flag","key":"deleted-flag","version":99}}`)
	sb.WriteString(`,{"event":"payload-transferred","data":{"state":"p1:10","version":10}}`)
	sb.WriteString(`]}`)
	return []byte(sb.String())
}

func TestParsePollingPayloadFullTransfer(t *testing.T) {
	body := makePollingBody(t, 2)
	changeSet, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)

	assert.Equal(t, subsystems.IntentTransferFull, changeSet.IntentCode())
	assert.Equal(t, "p1:10", changeSet.Selector().State())
	assert.Equal(t, 10, changeSet.Selector().Version())

	changes := changeSet.Changes()
	require.Len(t, changes, 4) // 2 flags + 1 segment + 1 delete
	assert.Equal(t, subsystems.ChangeTypePut, changes[0].Action)
	assert.Equal(t, "flag-0", changes[0].Key)
	assert.JSONEq(t, string(makeTestFlagJSON(t, "flag-0", 1)), string(changes[0].Object))
	assert.Equal(t, subsystems.ChangeTypeDelete, changes[3].Action)
	assert.Equal(t, "deleted-flag", changes[3].Key)

	collections, err := changeSet.Collections()
	require.NoError(t, err)
	itemsByKind := map[string]int{}
	for _, coll := range collections {
		itemsByKind[coll.Kind.GetName()] = len(coll.Items)
	}
	assert.Equal(t, map[string]int{"features": 3, "segments": 1}, itemsByKind) // 2 puts + 1 tombstone
	for _, coll := range collections {
		if coll.Kind.GetName() != "features" {
			continue
		}
		for _, item := range coll.Items {
			if item.Key == "deleted-flag" {
				assert.Nil(t, item.Item.Item)
				assert.Equal(t, 99, item.Item.Version)
			} else {
				assert.NotNil(t, item.Item.Item)
			}
		}
	}
}

func TestParsePollingPayloadIntentNone(t *testing.T) {
	body := []byte(`{"events":[{"event":"server-intent","data":{"payloads":[{"id":"p1","target":1,"intentCode":"none","reason":"up-to-date"}]}}]}`)
	changeSet, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)
	assert.Equal(t, subsystems.IntentNone, changeSet.IntentCode())
	assert.Empty(t, changeSet.Changes())
}

func TestParsePollingPayloadNoKnownEventsIsError(t *testing.T) {
	for name, body := range map[string]string{
		"empty events":        `{"events":[]}`,
		"no events property":  `{"other":true}`,
		"only unknown events": `{"events":[{"event":"future-event","data":{"a":1}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parsePollingPayload(context.Background(), []byte(body))
			assert.Error(t, err)
		})
	}
}

func TestParsePollingPayloadMalformedBody(t *testing.T) {
	for _, body := range []string{
		`{`,
		`[]`,
		`{"events":[{"event":"server-intent","data":{"payloads":[{}]}}`,
		`{"events":[{"event":"put-object","data":{"kind":"flag","key":"k","version":1,"object":{"key":}}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			_, err := parsePollingPayload(context.Background(), []byte(body))
			assert.Error(t, err)
		})
	}
}

func TestParsePollingPayloadUnknownEventsAreIgnored(t *testing.T) {
	body := []byte(`{"events":[` +
		`{"event":"future-event","data":{"whatever":[1,2,3]}},` +
		`{"event":"server-intent","data":{"payloads":[{"id":"p1","target":1,"intentCode":"xfer-full","reason":"r"}]}},` +
		`{"event":"another-future-event","data":"scalar data"},` +
		`{"event":"payload-transferred","data":{"state":"s","version":1}}` +
		`]}`)
	changeSet, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)
	assert.Equal(t, subsystems.IntentTransferFull, changeSet.IntentCode())
	assert.Equal(t, "s", changeSet.Selector().State())
}

func TestParsePollingPayloadDataBeforeEventName(t *testing.T) {
	flagJSON := makeTestFlagJSON(t, "flagkey", 1)
	body := []byte(fmt.Sprintf(`{"events":[`+
		`{"data":{"payloads":[{"id":"p1","target":1,"intentCode":"xfer-full","reason":"r"}]},"event":"server-intent"},`+
		`{"data":{"kind":"flag","key":"flagkey","version":1,"object":%s},"event":"put-object"},`+
		`{"data":{"state":"s","version":1},"event":"payload-transferred"}`+
		`]}`, flagJSON))
	changeSet, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)
	changes := changeSet.Changes()
	require.Len(t, changes, 1)
	assert.Equal(t, "flagkey", changes[0].Key)
	assert.JSONEq(t, string(flagJSON), string(changes[0].Object))
	collections, err := changeSet.Collections()
	require.NoError(t, err)
	require.Len(t, collections, 1)
	require.Len(t, collections[0].Items, 1)
	assert.NotNil(t, collections[0].Items[0].Item.Item)
}

func TestParsePollingPayloadEventWithoutDataIsError(t *testing.T) {
	body := []byte(`{"events":[` +
		`{"event":"server-intent","data":{"payloads":[{"id":"p1","target":1,"intentCode":"xfer-full","reason":"r"}]}},` +
		`{"event":"put-object"},` +
		`{"event":"payload-transferred","data":{"state":"s","version":1}}` +
		`]}`)
	_, err := parsePollingPayload(context.Background(), body)
	assert.Error(t, err)
}

func TestParsePollingPayloadEventsAfterTransferAreIgnored(t *testing.T) {
	// The trailing put-object is valid JSON but is never dispatched, because the payload is
	// already complete. (The default build's single-pass parser stops reading the body entirely
	// at that point; the easyjson build's envelope decode still requires the remainder to be
	// well-formed JSON, which is why this event must be syntactically valid.)
	body := []byte(`{"events":[` +
		`{"event":"server-intent","data":{"payloads":[{"id":"p1","target":1,"intentCode":"xfer-full","reason":"r"}]}},` +
		`{"event":"payload-transferred","data":{"state":"s","version":1}},` +
		`{"event":"put-object","data":{"kind":"flag","key":"never-dispatched","version":1,"object":{"key":12345}}}` +
		`]}`,
	)
	changeSet, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)
	assert.Empty(t, changeSet.Changes())
}

func TestParsePollingPayloadHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := parsePollingPayload(ctx, makePollingBody(t, 2))
	assert.ErrorIs(t, err, context.Canceled)
}

// parsePollingPayloadReflectionReference replicates the previous reflection-based parse
// (PollingPayload envelope unmarshal, per-event unmarshal, raw puts) so that tests and
// benchmarks can verify the single-pass parser against it.
func parsePollingPayloadReflectionReference(body []byte) (*subsystems.ChangeSet, error) {
	var payload subsystems.PollingPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	builder := subsystems.NewChangeSetBuilder()
	for _, event := range payload.Events {
		switch event.Name {
		case subsystems.EventServerIntent:
			var serverIntent subsystems.ServerIntent
			if err := json.Unmarshal(event.Data, &serverIntent); err != nil {
				return nil, err
			}
			if serverIntent.Payload.Code == subsystems.IntentNone {
				return builder.NoChanges(), nil
			}
			builder.Start(serverIntent)
		case subsystems.EventPutObject:
			var put subsystems.PutObject
			if err := json.Unmarshal(event.Data, &put); err != nil {
				return nil, err
			}
			builder.AddPut(put.Kind, put.Key, put.Version, put.Object)
		case subsystems.EventDeleteObject:
			var deleteObject subsystems.DeleteObject
			if err := json.Unmarshal(event.Data, &deleteObject); err != nil {
				return nil, err
			}
			builder.AddDelete(deleteObject.Kind, deleteObject.Key, deleteObject.Version)
		case subsystems.EventPayloadTransferred:
			var selector subsystems.Selector
			if err := json.Unmarshal(event.Data, &selector); err != nil {
				return nil, err
			}
			return builder.Finish(selector)
		}
	}
	return nil, fmt.Errorf("didn't receive any known protocol events in polling payload")
}

func TestParsePollingPayloadMatchesReflectionReference(t *testing.T) {
	body := makePollingBody(t, 20)

	got, err := parsePollingPayload(context.Background(), body)
	require.NoError(t, err)
	want, err := parsePollingPayloadReflectionReference(body)
	require.NoError(t, err)

	assert.Equal(t, want.IntentCode(), got.IntentCode())
	assert.Equal(t, want.Selector(), got.Selector())

	wantChanges, gotChanges := want.Changes(), got.Changes()
	require.Equal(t, len(wantChanges), len(gotChanges))
	for i := range wantChanges {
		assert.Equal(t, wantChanges[i].Action, gotChanges[i].Action, "change %d", i)
		assert.Equal(t, wantChanges[i].Kind, gotChanges[i].Kind, "change %d", i)
		assert.Equal(t, wantChanges[i].Key, gotChanges[i].Key, "change %d", i)
		assert.Equal(t, wantChanges[i].Version, gotChanges[i].Version, "change %d", i)
		assert.Equal(t, string(wantChanges[i].Object), string(gotChanges[i].Object), "change %d", i)
	}

	wantCollections, err := want.Collections()
	require.NoError(t, err)
	gotCollections, err := got.Collections()
	require.NoError(t, err)
	wantItems := flattenCollectionsForComparison(wantCollections)
	gotItems := flattenCollectionsForComparison(gotCollections)
	assert.Equal(t, wantItems, gotItems)
}

func flattenCollectionsForComparison(collections []ldstoretypes.Collection) map[string]ldstoretypes.ItemDescriptor {
	result := map[string]ldstoretypes.ItemDescriptor{}
	for _, coll := range collections {
		for _, item := range coll.Items {
			result[coll.Kind.GetName()+"/"+item.Key] = item.Item
		}
	}
	return result
}
