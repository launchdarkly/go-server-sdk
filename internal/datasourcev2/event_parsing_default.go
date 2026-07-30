//go:build !launchdarkly_easyjson
// +build !launchdarkly_easyjson

package datasourcev2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// JSON property names used by the single-pass event decoders.
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

// readInt reads a JSON number that must fit in a Go int, with the same acceptance rules as decoding
// into an int via encoding/json (the reflection decode used by the launchdarkly_easyjson build for
// the plain-int fields), so the two build variants stay in agreement. jreader.Int cannot be used:
// it coerces via int(Float64()), which silently truncates a fractional number (1.9 -> 1), loses
// precision beyond 2^53, and is implementation-defined out of range. Parsing the raw number literal
// instead means a fractional value (1.9), exponent notation (1e2), or an out-of-range value is
// rejected, large integers up to the int range are preserved exactly, and a JSON null yields the
// zero value -- exactly as encoding/json does.
func readInt(r *jreader.Reader) int {
	raw := r.RawValue()
	if r.Error() != nil {
		return 0
	}
	if string(raw) == "null" {
		// encoding/json leaves the zero value for a null; match that rather than erroring.
		return 0
	}
	n, err := strconv.ParseInt(string(raw), 10, 0)
	if err != nil {
		r.AddError(jreader.SyntaxError{Message: "expected integer"})
		return 0
	}
	return int(n)
}

// skipValue discards the next JSON value. It deliberately uses RawValue rather than
// jreader.SkipValue: SkipValue recurses one stack frame per level of nesting with no depth limit,
// so a deeply nested value in an unknown field of untrusted input could overflow the stack and
// crash the process. RawValue scans the value iteratively and validates it with encoding/json,
// whose nesting is capped -- turning an over-deep value into an error rather than a crash, matching
// the encoding/json decode the SDK used before this parser.
func skipValue(r *jreader.Reader) {
	_ = r.RawValue()
}

// This file contains the single-pass payload decoders used by the default build. Each event --
// and in particular each put-object's item JSON -- is scanned only once: scalars are read in
// place, and a recognized item's model is decoded directly from the stream while its raw bytes
// are captured as a zero-copy slice of the input by recording the reader's offset around the
// decode (the RFC 8259 compliant tokenizer fully validates everything it parses, so the captured
// span is known-valid JSON). Only values that are never model-parsed here -- objects of
// unrecognized kinds, and event data that arrives before its event name -- go through
// jreader.RawValue, which performs its own boundary scan and validation.

// parsePollingPayload walks a polling response body of the form {"events":[...]} in a single
// jsonstream pass and returns the completed change set. It mirrors the event semantics of the
// streaming data source: a server-intent of "none" short-circuits to a no-changes result, and a
// payload-transferred event completes the change set. Unknown event names are ignored for
// forwards compatibility.
//
// The returned ChangeSet's raw change objects reference the body slice, so the body must not be
// reused or modified while the ChangeSet is alive.
func parsePollingPayload(ctx context.Context, body []byte) (*subsystems.ChangeSet, error) {
	builder := subsystems.NewChangeSetBuilder()
	r := jreader.NewReader(body)
	for topObj := r.Object(); topObj.Next(); {
		if string(topObj.Name()) != propEvents {
			skipValue(&r)
			continue
		}
		for arr := r.Array(); arr.Next(); {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			changeSet, done, err := readPollingEvent(&r, builder)
			if err != nil {
				return nil, err
			}
			if done {
				return changeSet, nil
			}
		}
	}
	if err := r.Error(); err != nil {
		return nil, err
	}
	return nil, errors.New(errNoKnownPollingEvents)
}

// readPollingEvent consumes one {"event":...,"data":...} object from the events array. When the
// event completes the payload (server-intent "none" or payload-transferred), it returns the
// resulting change set with done set to true.
//
// The event's data is captured verbatim and decoded once the whole event object has been read.
// This keeps the decode independent of property order (the event name may follow the data) and
// makes a duplicated data property resolve last-wins, exactly as the reflection-based decode does.
func readPollingEvent(
	r *jreader.Reader,
	builder *subsystems.ChangeSetBuilder,
) (changeSet *subsystems.ChangeSet, done bool, err error) {
	var name subsystems.EventName
	var data []byte
	gotData := false
	for obj := r.Object(); obj.Next(); {
		switch string(obj.Name()) {
		case propEvent:
			name = subsystems.EventName(r.String())
		case propData:
			gotData = true
			data = r.RawValue()
		default:
			skipValue(r)
		}
	}
	if err := r.Error(); err != nil {
		return nil, false, err
	}
	if !gotData {
		switch name {
		case subsystems.EventServerIntent, subsystems.EventPutObject,
			subsystems.EventDeleteObject, subsystems.EventPayloadTransferred:
			// A payload-affecting event without data cannot be applied; treat the payload as
			// malformed rather than silently dropping the event.
			return nil, false, fmt.Errorf("polling payload event %q has no data", name)
		}
		return nil, false, nil
	}

	dataReader := jreader.NewReader(data)
	switch name {
	case subsystems.EventServerIntent:
		intent, err := readServerIntent(&dataReader)
		if err != nil {
			return nil, false, err
		}
		if intent.Payload.Code == subsystems.IntentNone {
			return builder.NoChanges(), true, nil
		}
		builder.Start(intent)
	case subsystems.EventPutObject:
		put, err := readPutObject(&dataReader, data)
		if err != nil {
			return nil, false, err
		}
		put.addTo(builder)
	case subsystems.EventDeleteObject:
		deleteObject, err := readDeleteObject(&dataReader)
		if err != nil {
			return nil, false, err
		}
		builder.AddDelete(deleteObject.Kind, deleteObject.Key, deleteObject.Version)
	case subsystems.EventPayloadTransferred:
		selector, err := readSelector(&dataReader)
		if err != nil {
			return nil, false, err
		}
		finished, err := builder.Finish(selector)
		if err != nil {
			return nil, false, err
		}
		return finished, true, nil
	default:
		// An unknown event name is ignored for forwards compatibility. RawValue already validated
		// the data above.
	}
	return nil, false, nil
}

// parsePutObjectEventData decodes the data of a put-object event in a single pass, capturing the
// item's raw JSON and its parsed form together.
func parsePutObjectEventData(data []byte) (parsedPutObject, error) {
	r := jreader.NewReader(data)
	p, err := readPutObject(&r, data)
	if err == nil {
		err = r.Error()
	}
	return p, err
}

// jsonWhitespace is the set of insignificant whitespace bytes RFC 8259 allows between tokens; a
// span captured via reader offsets may include them around the value and they are trimmed off.
const jsonWhitespace = " \t\r\n"

// readPutObject decodes a put-object event's data from r, which must have been created over
// input (offset-based span capture slices it).
func readPutObject(r *jreader.Reader, input []byte) (parsedPutObject, error) {
	var p parsedPutObject
	var object json.RawMessage
	// decodedKind records the kind under which the item was eagerly model-decoded while reading
	// the object, so that a "kind" property appearing after "object" (non-conformant, but json's
	// last-wins behavior handles it) can be detected and the item re-parsed under the final kind
	// rather than left mis-typed.
	var decodedKind subsystems.ObjectKind
	for obj := r.Object(); obj.Next(); {
		switch string(obj.Name()) {
		case propKind:
			p.kind = subsystems.ObjectKind(r.String())
		case propKey:
			p.key = r.String()
		case propVersion:
			p.version = readInt(r)
		case propObject:
			if kind, recognized := p.kind.ToFDV1(); recognized {
				// The kind is already known (LaunchDarkly services always write "kind" before
				// "object"), so the item is model-decoded directly from the stream -- which
				// fully validates it -- while the reader offsets around the decode capture the
				// value's raw bytes with no additional scan.
				start := r.Offset()
				item, err := kind.DeserializeFromJSONReader(r)
				if err != nil {
					return p, err
				}
				span := bytes.Trim(input[start:r.Offset()], jsonWhitespace)
				object = json.RawMessage(span[:len(span):len(span)])
				p.item, p.hasItem, decodedKind = item, true, p.kind
			} else {
				// The kind is unrecognized or has not been read yet, so the value cannot be
				// model-decoded here; RawValue captures it with full validation, and if the
				// kind turns out to be recognized it is deserialized after the loop.
				object = json.RawMessage(r.RawValue())
			}
		default:
			skipValue(r)
		}
	}
	if err := r.Error(); err != nil {
		return p, err
	}
	p.object = object
	if object == nil {
		return p, nil
	}
	kind, recognized := p.kind.ToFDV1()
	if !recognized {
		// The final kind is unrecognized -- either it was never a known kind, or a later "kind"
		// property overrode an earlier recognized one. Keep the object raw and unparsed for
		// forwards compatibility, discarding any item eagerly decoded under the earlier kind. The
		// bytes were already validated (by the eager decode or by RawValue).
		p.hasItem = false
		return p, nil
	}
	if p.hasItem && decodedKind == p.kind {
		// The item was already decoded under the final kind while reading the object.
		return p, nil
	}
	// The object was captured before its kind was known, or a later "kind" changed the type after
	// the object was decoded; parse the captured bytes under the final kind so that the stored item
	// and kind always agree.
	itemReader := jreader.NewReader(object)
	item, err := kind.DeserializeFromJSONReader(&itemReader)
	if err != nil {
		return p, err
	}
	p.item, p.hasItem = item, true
	return p, nil
}

// parseServerIntentEventData decodes the data of a server-intent event. The intent is required to
// have at least one payload (at index 0) at this time, matching subsystems.ServerIntent's decode.
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
			skipValue(r)
			continue
		}
		for arr := r.Array(); arr.Next(); {
			// The protocol allows more than one payload, but SDKs currently only support one;
			// any additional payloads are skipped.
			if gotPayload {
				skipValue(r)
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
					skipValue(r)
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
			skipValue(r)
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
			// subsystems.Selector's decode (the reflection reference for this event) reads version
			// through float64 and truncates, so use the lenient jreader.Int here rather than the
			// strict readInt used for the other integer fields, keeping the two builds in
			// agreement. The value is deprecated and not used by consumers.
			version = r.Int()
			gotVersion = true
		default:
			skipValue(r)
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
			skipValue(&r)
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
			skipValue(&r)
		}
	}
	return errorData, r.Error()
}
