package datasource

import (
	"errors"
	"strings"

	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/datakinds"

	"gopkg.in/launchdarkly/go-jsonstream.v1/jreader"
)

var (
	putDataRequiredProperties    = []string{"data"}            //nolint:gochecknoglobals
	patchDataRequiredProperties  = []string{"path", "data"}    //nolint:gochecknoglobals
	deleteDataRequiredProperties = []string{"path", "version"} //nolint:gochecknoglobals
)

// This is the logical representation of the data in the "put" event. In the JSON representation,
// the "data" property is actually a map of maps, but the schema we use internally is a list of
// lists instead.
//
// The "path" property is normally always "/"; the LD streaming service sends this property, but
// some versions of Relay do not, so we do not require it.
//
// Example JSON representation:
//
// {
//   "path": "/",
//   "data": {
//     "flags": {
//       "flag1": { "key": "flag1", "version": 1, ...etc. },
//       "flag2": { "key": "flag2", "version": 1, ...etc. },
//     },
//     "segments": {
//       "segment1": { "key", "segment1", "version": 1, ...etc. }
//     }
//   }
// }
type putData struct {
	Path string // we don't currently do anything with this
	Data []ldstoretypes.Collection
}

// This is the logical representation of the data in the "patch" event. In the JSON representation,
// there is a "path" property in the format "/flags/key" or "/segments/key", which we convert into
// Kind and Key when we parse it. The "data" property is the JSON representation of the flag or
// segment, which we deserialize into an ItemDescriptor.
//
// Example JSON representation:
//
// {
//   "path": "/flags/flagkey",
//   "data": {
//     "key": "flagkey",
//     "version": 2, ...etc.
//   }
// }
type patchData struct {
	Kind ldstoretypes.DataKind
	Key  string
	Data ldstoretypes.ItemDescriptor
}

// This is the logical representation of the data in the "delete" event. In the JSON representation,
// there is a "path" property in the format "/flags/key" or "/segments/key", which we convert into
// Kind and Key when we parse it.
//
// Example JSON representation:
//
// {
//   "path": "/flags/flagkey",
//   "version": 3
// }
type deleteData struct {
	Kind    ldstoretypes.DataKind
	Key     string
	Version int
}

func parsePutData(data []byte) (putData, error) {
	var ret putData
	r := jreader.NewReader(data)
	for obj := r.Object().WithRequiredProperties(putDataRequiredProperties); obj.Next(); {
		switch string(obj.Name()) {
		case "path": //nolint:goconst // linter wants us to define constants, but that makes code like this less clear
			ret.Path = r.String()
		case "data": //nolint:goconst
			ret.Data = parseAllStoreDataFromJSONReader(&r)
		}
	}
	return ret, r.Error()
}

func parsePatchData(data []byte) (patchData, error) {
	var ret patchData
	r := jreader.NewReader(data)
	var kind datakinds.DataKindInternal
	var key string
	parseItem := func() (patchData, error) {
		item, err := kind.DeserializeFromJSONReader(&r)
		if err != nil {
			return patchData{}, err
		}
		ret.Data = item
		return ret, nil
	}
	for obj := r.Object().WithRequiredProperties(patchDataRequiredProperties); obj.Next(); {
		switch string(obj.Name()) {
		case "path":
			path := r.String()
			kind, key = parsePath(path)
			ret.Kind, ret.Key = kind, key
			if kind == nil {
				// An unrecognized path isn't considered an error; we'll just return a nil kind,
				// indicating that we should ignore this event.
				return ret, nil
			}
		case "data":
			if kind != nil {
				// If kind is nil, it means we happened to read the "data" property before the
				// "path" property, so we don't yet know what kind of data model object this is,
				// so we can't parse it yet and we'll have to do a second pass.
				return parseItem()
			}
		}
	}
	if err := r.Error(); err != nil {
		return patchData{}, err
	}
	// If we got here, it means we couldn't parse the data model object yet because we saw the
	// "data" property first. But we definitely saw both properties (otherwise we would've got
	// an error due to using WithRequiredProperties) so kind is now non-nil.
	r = jreader.NewReader(data)
	for obj := r.Object(); obj.Next(); {
		if string(obj.Name()) == "data" {
			return parseItem()
		}
	}
	if r.Error() != nil {
		return patchData{}, r.Error()
	}
	return patchData{}, errors.New("patch event had no data property")
}

func parseDeleteData(data []byte) (deleteData, error) {
	var ret deleteData
	r := jreader.NewReader(data)
	for obj := r.Object().WithRequiredProperties(deleteDataRequiredProperties); obj.Next(); {
		switch string(obj.Name()) {
		case "path":
			path := r.String()
			ret.Kind, ret.Key = parsePath(path)
			if ret.Kind == nil {
				// An unrecognized path isn't considered an error; we'll just return a nil kind,
				// indicating that we should ignore this event.
				return ret, nil
			}
		case "version":
			ret.Version = r.Int()
		}
	}
	if r.Error() != nil {
		return deleteData{}, r.Error()
	}
	return ret, nil
}

func parsePath(path string) (datakinds.DataKindInternal, string) {
	switch {
	case strings.HasPrefix(path, "/segments/"):
		return datakinds.Segments, strings.TrimPrefix(path, "/segments/")
	case strings.HasPrefix(path, "/flags/"):
		return datakinds.Features, strings.TrimPrefix(path, "/flags/")
	default:
		return nil, ""
	}
}
