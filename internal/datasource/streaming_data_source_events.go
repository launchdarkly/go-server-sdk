package datasource

import (
	"errors"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"strings"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
)

var (
	putDataRequiredProperties    = []string{"data"}            //nolint:gochecknoglobals
	patchDataRequiredProperties  = []string{"path", "data"}    //nolint:gochecknoglobals
	deleteDataRequiredProperties = []string{"path", "version"} //nolint:gochecknoglobals
)

func parsePutData(data []byte) (fdv2proto.PutData, error) {
	var ret fdv2proto.PutData
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

func parsePatchData(data []byte) (fdv2proto.PatchData, error) {
	var ret fdv2proto.PatchData
	r := jreader.NewReader(data)
	var kind datakinds.DataKindInternal
	var key string
	parseItem := func() (fdv2proto.PatchData, error) {
		item, err := kind.DeserializeFromJSONReader(&r)
		if err != nil {
			return fdv2proto.PatchData{}, err
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
		return fdv2proto.PatchData{}, err
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
		return fdv2proto.PatchData{}, r.Error()
	}
	return fdv2proto.PatchData{}, errors.New("patch event had no data property")
}

func parseDeleteData(data []byte) (fdv2proto.DeleteData, error) {
	var ret fdv2proto.DeleteData
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
		return fdv2proto.DeleteData{}, r.Error()
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
