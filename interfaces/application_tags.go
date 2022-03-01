package interfaces

import "regexp"

var validTagKeyOrValueRegex = regexp.MustCompile(`^[\w.-]*$`)

// ApplicationTags is an immutable container for any values defined in Config.Tags. Applications will
// normally never need to reference this type, but the SDK passes it to subcomponents as part of
// BasicConfiguration.
type ApplicationTags struct {
	all map[string][]string
}

// Keys returns a new string slice containing all distinct tag keys.
func (t ApplicationTags) Keys() []string {
	if len(t.all) == 0 {
		return nil
	}
	ret := make([]string, 0, len(t.all))
	for k := range t.all {
		ret = append(ret, k)
	}
	return ret
}

// Values returns a new string slice containing all values for this tag key, or nil if none.
func (t ApplicationTags) Values(key string) []string {
	values := t.all[key]
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

// NewApplicationTags copies the specified tag keys and values into an ApplicationTags struct, while
// discarding any keys or values that contain invalid characters, so that the returned struct is
// guaranteed to contain only valid strings. The second return value is true if all strings were
// valid or false if some were invalid.
func NewApplicationTags(fromMap map[string][]string) (ApplicationTags, bool) {
	if len(fromMap) == 0 {
		return ApplicationTags{}, true
	}
	outMap := make(map[string][]string)
	allValid := true
	for k, vv := range fromMap {
		if !isValidTagKeyOrValue(k) {
			allValid = false
			continue
		}
		outSlice := make([]string, 0, len(vv))
		for _, v := range vv {
			if !isValidTagKeyOrValue(v) {
				allValid = false
				continue
			}
			outSlice = append(outSlice, v)
		}
		outMap[k] = outSlice
	}
	return ApplicationTags{all: outMap}, allValid
}

func isValidTagKeyOrValue(s string) bool {
	return validTagKeyOrValueRegex.MatchString(s)
}
