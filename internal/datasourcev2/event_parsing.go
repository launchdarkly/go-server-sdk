package datasourcev2

import (
	"encoding/json"
	"errors"
	"math"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// This file contains the decoders for the FDv2 protocol events that are shared by both build
// variants. The payload-level decoders -- the ones responsible for capturing each put-object
// item's raw JSON alongside its parsed form -- are split by build tag:
//
//   - event_parsing_default.go: a single-pass jsonstream decoder using jreader.RawValue for
//     zero-copy raw capture. Each event, and in particular each item's JSON, is scanned once.
//   - event_parsing_easyjson.go: the previous reflection-based decode (kept for the
//     launchdarkly_easyjson build so that no new code depends on the easyjson token reader,
//     which is planned for removal).
//
// Both variants produce the same results: a parsedPutObject carrying the item's raw bytes and,
// for recognized kinds, its parsed form, so that the ChangeSet's collections are assembled
// without a later re-parse. The decoders are tolerant of property ordering: nothing here assumes
// that, for example, "kind" appears before "object" in a put-object event, even though
// LaunchDarkly services always write them in that order.

// JSON property names shared by the FDv2 protocol event decoders.
const (
	propEvents     = "events"
	propEvent      = "event"
	propData       = "data"
	propKind       = "kind"
	propKey        = "key"
	propVersion    = "version"
	propObject     = "object"
	propReason     = "reason"
	propState      = "state"
	propPayloads   = "payloads"
	propID         = "id"
	propTarget     = "target"
	propIntentCode = "intentCode"
	propPayloadID  = "payloadId"
)

const errNoKnownPollingEvents = "didn't receive any known protocol events in polling payload"

// readInt reads a JSON number that must be an integer. jreader.Int coerces via int(Float64()),
// which silently truncates a fractional number (for example 1.9 becomes 1). The FDv2 protocol's
// version and target fields are integers, so a fractional value is a malformed payload and is
// rejected here, matching the reflection-based decode used by the launchdarkly_easyjson build.
func readInt(r *jreader.Reader) int {
	f := r.Float64()
	if f != math.Trunc(f) {
		r.AddError(jreader.SyntaxError{Message: "expected integer, got fractional number"})
		return 0
	}
	return int(f)
}

// parsedPutObject is the result of decoding a put-object event's data.
type parsedPutObject struct {
	kind    subsystems.ObjectKind
	key     string
	version int
	// object is the item's raw JSON. In the default build it references the input buffer passed
	// to parsePutObjectEventData and is valid only as long as that buffer is.
	object json.RawMessage
	// item is the parsed representation of object, set only when hasItem is true. hasItem is
	// false when the kind is unrecognized, in which case the raw bytes are still retained for
	// forwards compatibility.
	item    ldstoretypes.ItemDescriptor
	hasItem bool
}

// addTo adds the put to a change-set builder, carrying the parsed item along when there is one.
func (p parsedPutObject) addTo(builder *subsystems.ChangeSetBuilder) {
	if p.hasItem {
		builder.AddParsedPut(p.kind, p.key, p.version, p.object, p.item)
	} else {
		builder.AddPut(p.kind, p.key, p.version, p.object)
	}
}

// parseServerIntentEventData decodes the data of a server-intent event. The intent is required to
// have at least one payload (at index 0) at this time.
func parseServerIntentEventData(data []byte) (subsystems.ServerIntent, error) {
	r := jreader.NewReader(data)
	intent, err := readServerIntent(&r)
	if err == nil {
		err = r.Error()
	}
	return intent, err
}

func readServerIntent(r *jreader.Reader) (subsystems.ServerIntent, error) {
	var intent subsystems.ServerIntent
	gotPayload := false
	for obj := r.Object(); obj.Next(); {
		if string(obj.Name()) != propPayloads {
			_ = r.SkipValue()
			continue
		}
		for arr := r.Array(); arr.Next(); {
			// The protocol allows more than one payload, but SDKs currently only support one;
			// any additional payloads are skipped.
			if gotPayload {
				_ = r.SkipValue()
				continue
			}
			for payloadObj := r.Object(); payloadObj.Next(); {
				switch string(payloadObj.Name()) {
				case propID:
					intent.Payload.ID = r.String()
				case propTarget:
					intent.Payload.Target = readInt(r)
				case propIntentCode:
					intent.Payload.Code = subsystems.IntentCode(r.String())
				case propReason:
					intent.Payload.Reason = r.String()
				default:
					_ = r.SkipValue()
				}
			}
			gotPayload = true
		}
	}
	if err := r.Error(); err != nil {
		return intent, err
	}
	if !gotPayload {
		// It is a protocol error for the payload list to be missing or empty.
		return intent, errors.New("changeset: server-intent event has no payloads")
	}
	return intent, nil
}

// parseDeleteObjectEventData decodes the data of a delete-object event.
func parseDeleteObjectEventData(data []byte) (subsystems.DeleteObject, error) {
	r := jreader.NewReader(data)
	deleteObject, err := readDeleteObject(&r)
	if err == nil {
		err = r.Error()
	}
	return deleteObject, err
}

func readDeleteObject(r *jreader.Reader) (subsystems.DeleteObject, error) {
	var d subsystems.DeleteObject
	for obj := r.Object(); obj.Next(); {
		switch string(obj.Name()) {
		case propKind:
			d.Kind = subsystems.ObjectKind(r.String())
		case propKey:
			d.Key = r.String()
		case propVersion:
			d.Version = readInt(r)
		default:
			_ = r.SkipValue()
		}
	}
	return d, r.Error()
}

// parseSelectorEventData decodes the data of a payload-transferred event. Both the state and the
// version are required.
func parseSelectorEventData(data []byte) (subsystems.Selector, error) {
	r := jreader.NewReader(data)
	selector, err := readSelector(&r)
	if err == nil {
		err = r.Error()
	}
	return selector, err
}

func readSelector(r *jreader.Reader) (subsystems.Selector, error) {
	var state string
	var version int
	gotState, gotVersion := false, false
	for obj := r.Object(); obj.Next(); {
		switch string(obj.Name()) {
		case propState:
			state = r.String()
			gotState = true
		case propVersion:
			version = readInt(r)
			gotVersion = true
		default:
			_ = r.SkipValue()
		}
	}
	if err := r.Error(); err != nil {
		return subsystems.NoSelector(), err
	}
	if !gotState {
		return subsystems.NoSelector(), errors.New("unmarshal selector: missing state field")
	}
	if !gotVersion {
		return subsystems.NoSelector(), errors.New("unmarshal selector: missing version field")
	}
	return subsystems.NewSelector(state, version), nil
}

// parseGoodbyeEventData decodes the data of a goodbye event.
func parseGoodbyeEventData(data []byte) (subsystems.Goodbye, error) {
	r := jreader.NewReader(data)
	var goodbye subsystems.Goodbye
	for obj := r.Object(); obj.Next(); {
		if string(obj.Name()) == propReason {
			goodbye.Reason = r.String()
		} else {
			_ = r.SkipValue()
		}
	}
	return goodbye, r.Error()
}

// parseErrorEventData decodes the data of an error event.
func parseErrorEventData(data []byte) (subsystems.Error, error) {
	r := jreader.NewReader(data)
	var errorData subsystems.Error
	for obj := r.Object(); obj.Next(); {
		switch string(obj.Name()) {
		case propPayloadID:
			errorData.PayloadID = r.String()
		case propReason:
			errorData.Reason = r.String()
		default:
			_ = r.SkipValue()
		}
	}
	return errorData, r.Error()
}
