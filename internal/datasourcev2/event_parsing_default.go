//go:build !launchdarkly_easyjson
// +build !launchdarkly_easyjson

package datasourcev2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

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
			_ = r.SkipValue()
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
			_ = r.SkipValue()
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
				p.item, p.hasItem = item, true
			} else {
				// The kind is unrecognized or has not been read yet, so the value cannot be
				// model-decoded here; RawValue captures it with full validation, and if the
				// kind turns out to be recognized it is deserialized after the loop.
				object = json.RawMessage(r.RawValue())
			}
		default:
			_ = r.SkipValue()
		}
	}
	if err := r.Error(); err != nil {
		return p, err
	}
	p.object = object
	if p.hasItem || object == nil {
		return p, nil
	}
	if kind, recognized := p.kind.ToFDV1(); recognized {
		// The object appeared before the kind; parse it now.
		itemReader := jreader.NewReader(object)
		item, err := kind.DeserializeFromJSONReader(&itemReader)
		if err != nil {
			return p, err
		}
		p.item, p.hasItem = item, true
	}
	// If the kind is unrecognized, the object is kept raw (and unparsed) for forwards
	// compatibility. No further validation is needed: RawValue already fully validated the
	// bytes, so a malformed object has failed the payload above regardless of kind.
	return p, nil
}
