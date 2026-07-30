package datasourcev2

import (
	"encoding/json"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// This file contains the parts of FDv2 protocol event decoding that are shared by both build
// variants. The event decoders themselves are split by build tag:
//
//   - event_parsing_default.go: a single-pass jsonstream decoder using jreader.RawValue/Offset for
//     zero-copy raw capture. Each event, and in particular each put-object item's JSON, is scanned
//     once. Its scalar decoders are written to match encoding/json's acceptance rules (see below).
//   - event_parsing_easyjson.go: a reflection-based decode (encoding/json) for the
//     launchdarkly_easyjson build, so that no new code depends on the easyjson token reader, which
//     is planned for removal from go-jsonstream.
//
// The two variants accept and reject the same payloads. The easyjson build decodes each scalar
// event with encoding/json into the subsystems types; the default build's jsonstream decoders are
// written to mirror that behavior exactly -- including rejecting non-integer version/target values
// on the plain-int types and matching Selector's float64-based (truncating) version handling. Both
// produce a parsedPutObject carrying the item's raw bytes and, for recognized kinds, its parsed
// form, so the ChangeSet's collections are assembled without a later re-parse.

const errNoKnownPollingEvents = "didn't receive any known protocol events in polling payload"

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
