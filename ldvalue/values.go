// Package ldvalue provides an abstraction of the LaunchDarkly SDK's general value type. LaunchDarkly
// supports the standard JSON data types of null, boolean, number, string, array, and object (map), for
// any feature flag variation or custom user attribute. The ldvalue.Value type can contain any of these.
//
// For backward compatibility, some SDK methods and types still represent these values with the general
// Go type "interface{}". Whenever there is an alternative that uses ldvalue.Value, that is preferred;
// in the next major version release of the SDK, the uses of interface{} will be removed. There are two
// reasons. First, interface{} can contain values that have no JSON encoding. Second, interface{} could
// be an array, slice, or map, all of which are passed by reference and could be modified in one place
// causing unexpected effects in another place. Value is guaranteed to be immutable and to contain only
// JSON-compatible types as long as you do not use the UnsafeValueCopy() and UnsafeInnerValue() methods
// (which will be removed in the future).
package ldvalue

import (
	"encoding/json"
	"errors"
	"strconv"
)

// Notes for future implementation changes:
// In a future major version release, we will eliminate usage of interface{} as a flag value type and
// user custom attribute type in the public API. At that point, we can also make the following changes:
// - Replace interface{} with Value in all data model structs that are parsed from JSON (we may need to
//   find a better parser implementation for this).
// - Remove UnsafeValueCopy and UnsafeInnerValue.
// - Represent arrays and maps internally as []Value and map[string]Value, so we will not need to do
//   a deep copy when retrieving a value.

// Value represents any of the data types supported by JSON, all of which can be used for a LaunchDarkly
// feature flag variation or a custom user attribute.
type Value struct {
	// Note that the zero value of ValueType is NullType, so the zero of Value is a null value.
	valueType ValueType
	// Used when the value is a boolean.
	boolValue bool
	// Used when the value is a number.
	numberValue float64
	// Used when the value is a string.
	stringValue string
	// Representation of the value as an interface{}. If the value was originally produced from an
	// interface{}, then we use this; otherwise we try to avoid setting it because that requires a
	// heap allocation. For numeric types, we always store this as a float64 so struct equality will
	// work as expected.
	valueInstance interface{}
}

// ValueType indicates which JSON type is contained in a Value.
type ValueType int

const (
	// NullType describes a null value.
	NullType ValueType = iota
	// BoolType describes a boolean value.
	BoolType ValueType = iota
	// NumberType describes a numeric value. JSON does not have separate types for int and float, but
	// you can convert to either.
	NumberType ValueType = iota
	// StringType describes a string value.
	StringType ValueType = iota
	// ArrayType describes an array value.
	ArrayType ValueType = iota
	// ObjectType describes an object (a.k.a. map).
	ObjectType ValueType = iota
	// RawType describes a json.RawMessage value. This value will not be parsed or interpreted as
	// any other data type, and can be accessed only by calling Raw().
	RawType ValueType = iota
)

// Intern the following primitive values as interface{} so we don't reallocate them
var (
	zeroAsInterface        interface{} = float64(0)
	emptyStringAsInterface interface{} = ""
)

// ArrayBuilder is a builder created by ArrayBuild(), for creating immutable arrays.
type ArrayBuilder interface {
	// Add appends an element to the array builder.
	Add(value Value) ArrayBuilder
	// Build creates a Value containing the previously added array elements. Continuing to modify the
	// same builder by calling Add after that point does not affect the returned array.
	Build() Value
}

type arrayBuilderImpl struct {
	copyOnWrite bool
	output      []interface{}
}

// ObjectBuilder is a builder created by ObjectBuild(), for creating immutable JSON objects.
type ObjectBuilder interface {
	// Set sets a key-value pair in the object builder.
	Set(key string, value Value) ObjectBuilder
	// Build creates a Value containing the previously specified key-value pairs. Continuing to modify
	// the same builder by calling Set after that point does not affect the returned object.
	Build() Value
}

type objectBuilderImpl struct {
	copyOnWrite bool
	output      map[string]interface{}
}

// String returns the name of the value type.
func (t ValueType) String() string {
	switch t {
	case NullType:
		return "null"
	case BoolType:
		return "bool"
	case NumberType:
		return "number"
	case StringType:
		return "string"
	case ArrayType:
		return "array"
	case ObjectType:
		return "object"
	case RawType:
		return "raw"
	default:
		return "unknown"
	}
}

func toSafeValue(value interface{}) interface{} {
	switch o := value.(type) {
	case []interface{}:
		return deepCopyArray(o)
	case map[string]interface{}:
		return deepCopyMap(o)
	default:
		return value
	}
}

func deepCopyArray(a []interface{}) []interface{} {
	ret := make([]interface{}, len(a))
	for i, v := range a {
		ret[i] = toSafeValue(v)
	}
	return ret
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	ret := make(map[string]interface{}, len(m))
	for k, v := range m {
		ret[k] = toSafeValue(v)
	}
	return ret
}

func fromValue(valueAsInterface interface{}, deepCopy bool) Value {
	if valueAsInterface == nil {
		return Null()
	}
	switch o := valueAsInterface.(type) {
	case Value:
		return o
	case bool:
		return Bool(o)
	// Coerce all numbers to float64, so numerically identical values will always be equal in
	// a struct comparison
	case int8:
		return Float64(float64(o))
	case uint8:
		return Float64(float64(o))
	case int16:
		return Float64(float64(o))
	case uint16:
		return Float64(float64(o))
	case int:
		return Float64(float64(o))
	case uint:
		return Float64(float64(o))
	case int32:
		return Float64(float64(o))
	case uint32:
		return Float64(float64(o))
	case float32:
		return Float64(float64(o))
	case float64:
		return Value{valueType: NumberType, numberValue: o, valueInstance: valueAsInterface}
	case string:
		return Value{valueType: StringType, stringValue: o, valueInstance: valueAsInterface}
	case []interface{}:
		if deepCopy {
			return ArrayCopy(o)
		}
		return Value{valueType: ArrayType, valueInstance: valueAsInterface}
	case map[string]interface{}:
		if deepCopy {
			return ObjectCopy(o)
		}
		return Value{valueType: ObjectType, valueInstance: valueAsInterface}
	case json.RawMessage:
		return Value{valueType: RawType, valueInstance: valueAsInterface}
	default:
		// We should never see an unsupported type here, because this method is only called
		// with a value that was parsed as a generic interface{} by the JSON parser.
		return Null()
	}
}

// InnerValue converts the Value to its corresponding Go type as an interface{}. The returned
// value is nil if the Value is null; a bool, float64, or string if it is a primitive JSON
// type (note that all numbers are stored as float64); a slice of type []interface{} if it is
// an array; or a map of type map[string]interface{} if it is a JSON object.
//
// Slices and maps are deep-copied, which preserves immutability of the Value but may be an
// expensive operation. To examine array and object values without copying the whole data
// structure, use getter methods: Count, Keys, GetByIndex, TryGetByIndex, GetByKey, TryGetByKey.
func (v Value) InnerValue() interface{} {
	return toSafeValue(v.valueInstance)
}

// UnsafeInnerValue returns the actual Go value inside the Value. Application code should never
// use this method, since it can break the immutability contract of Value.
//
// This method and UnsafeValueCopy are provided for backward compatibility: some SDK methods
// still use interface{}, so the SDK needs to be able to convert between interface{} and Value
// without doing expensive deep-copies. In a future version of the SDK, they will be removed.
//
// Deprecated: Application code should always use InnerValue.
func (v Value) UnsafeInnerValue() interface{} {
	return v.valueInstance
}

// Null creates a null Value.
func Null() Value {
	return Value{valueType: NullType}
}

// Bool creates a boolean Value.
func Bool(value bool) Value {
	return Value{valueType: BoolType, boolValue: true, valueInstance: value}
}

// Int creates a numeric Value from an integer.
func Int(value int) Value {
	return Float64(float64(value))
}

// Float64 creates a numeric Value from a float64.
func Float64(value float64) Value {
	if value == 0 {
		return Value{valueType: NumberType, numberValue: 0, valueInstance: zeroAsInterface}
	}
	return Value{valueType: NumberType, numberValue: value, valueInstance: value}
}

// String creates a string Value.
func String(value string) Value {
	if value == "" {
		return Value{valueType: StringType, stringValue: "", valueInstance: emptyStringAsInterface}
	}
	return Value{valueType: StringType, stringValue: value, valueInstance: value}
}

// Raw creates an unparsed JSON Value.
func Raw(value json.RawMessage) Value {
	return Value{valueType: RawType, valueInstance: value}
}

// ValueCopy creates a Value from an arbitrary interface{} value of any type.
//
// If the value is nil, a boolean, an integer, a floating-point number, or a string, it becomes the
// corresponding JSON primitive value type. If it is a slice of values ([]interface{}), it is
// deep-copied to an array value (same as ArrayCopy). If it is a map of strings to values
// (map[string]interface{}), it is deep-copied to an object value (same as ObjectCopy). All other
// types return Null().
func ValueCopy(value interface{}) Value {
	return fromValue(value, true)
}

// UnsafeValueCopy creates a Value from a shallow copy of an arbitrary Go value. Application code
// should never use this method, since it can break the immutability contract of Value.
//
// This method and UnsafeInnerValue are provided for backward compatibility: some SDK methods
// still use interface{}, so the SDK needs to be able to convert between interface{} and Value
// without doing expensive deep-copies. In a future version of the SDK, they will be removed.
//
// Any value that is not assignable to a JSON primitive type, and is not []interface{} or
// map[string]interface{}, will be converted to Null().
//
// Deprecated: Application code should always use ValueCopy.
func UnsafeValueCopy(value interface{}) Value {
	return fromValue(value, false)
}

// ArrayCopy creates a Value by copying an existing slice.
//
// This is a deep copy, so any slices or maps contained in this slice can be modified later without
// affecting the returned Value. Values are converted using the same rules as ValueCopy.
func ArrayCopy(a []interface{}) Value {
	return Value{valueType: ArrayType, valueInstance: deepCopyArray(a)}
}

// ArrayBuild creates a builder for constructing an immutable array Value.
//
// The capacity parameter is the same as the capacity of a slice, allowing you to preallocate space
// if you know the number of elements; otherwise you can pass zero.
//
//     arrayValue := ldvalue.ArrayBuild(2).Add(ldvalue.Int(100)).Add(ldvalue.Int(200)).Build()
func ArrayBuild(capacity int) ArrayBuilder {
	return &arrayBuilderImpl{output: make([]interface{}, 0, capacity)}
}

func (b *arrayBuilderImpl) Add(value Value) ArrayBuilder {
	if b.copyOnWrite {
		b.output = deepCopyArray(b.output)
		b.copyOnWrite = false
	}
	b.output = append(b.output, value.valueInstance)
	return b
}

func (b *arrayBuilderImpl) Build() Value {
	b.copyOnWrite = true
	return Value{valueType: ArrayType, valueInstance: b.output}
}

// ObjectCopy creates a Value by copying an existing map.
//
// This is a deep copy, so any slices or maps contained in this map can be modified later without
// affecting the returned Value. Values are converted using the same rules as ValueCopy.
func ObjectCopy(m map[string]interface{}) Value {
	return Value{valueType: ObjectType, valueInstance: deepCopyMap(m)}
}

// ObjectBuild creates a builder for constructing an immutable JSON object Value.
//
// The capacity parameter is the same as the capacity of a map, allowing you to preallocate space
// if you know the number of elements; otherwise you can pass zero.
//
//     objValue := ldvalue.ObjectBuild(2).Set("a", ldvalue.Int(100)).Add("b", ldvalue.Int(200)).Build()
func ObjectBuild(capacity int) ObjectBuilder {
	return &objectBuilderImpl{output: make(map[string]interface{}, capacity)}
}

func (b *objectBuilderImpl) Set(name string, value Value) ObjectBuilder {
	if b.copyOnWrite {
		b.output = deepCopyMap(b.output)
		b.copyOnWrite = false
	}
	b.output[name] = value.valueInstance
	return b
}

func (b *objectBuilderImpl) Build() Value {
	b.copyOnWrite = true
	return Value{valueType: ObjectType, valueInstance: b.output}
}

// Type returns the ValueType of the Value.
func (v Value) Type() ValueType {
	return v.valueType
}

// IsNull returns true if the Value is a null.
func (v Value) IsNull() bool {
	return v.valueType == NullType
}

// IsNumber returns true if the Value is numeric.
func (v Value) IsNumber() bool {
	return v.valueType == NumberType
}

// IsInt returns true if the Value is an integer. JSON does not have separate types for integer and
// floating-point values; they are both just numbers. IsInt returns true if and only if the actual
// mnumeric value has no fractional component, so ldvalue.Int(2).IsInt() and ldvalue.Float64(2.0).IsInt()
// are both true.
func (v Value) IsInt() bool {
	if v.valueType == NumberType {
		return v.numberValue == float64(int(v.numberValue))
	}
	return false
}

// Bool returns the Value as a boolean. If the Value is not a boolean, it returns false.
func (v Value) Bool() bool {
	return v.valueType == BoolType && v.boolValue
}

// Int returns the value as an int. If the Value is not numeric, it returns zero. If the value is a
// number but not an integer, it is rounded toward zero (truncated).
func (v Value) Int() int {
	if v.valueType == NumberType {
		return int(v.numberValue)
	}
	return 0
}

// Float64 returns the value as a float64. If the Value is not numeric, it returns zero.
func (v Value) Float64() float64 {
	if v.valueType == NumberType {
		return v.numberValue
	}
	return 0
}

// String returns the value as a string. If the value is not a string, it returns an empty string.
func (v Value) String() string {
	if v.valueType == StringType {
		return v.stringValue
	}
	return ""
}

// JSONString returns the JSON representation of the value.
func (v Value) JSONString() string {
	switch v.valueType {
	case NullType:
		return "null"
	case BoolType:
		if v.boolValue {
			return "true"
		}
		return "false"
	case NumberType:
		if v.IsInt() {
			return strconv.Itoa(int(v.numberValue))
		}
		return strconv.FormatFloat(v.numberValue, 'f', -1, 64)
	default:
		bytes, err := json.Marshal(v.valueInstance)
		if err != nil {
			// It shouldn't be possible for marshalling to fail, because Value should only contain
			// JSON-compatible types. However, UnsafeValueCopy and UnsafeInnerValue do allow a
			// badly behaved application to put an incompatible type into an array or map. In this
			// case we simply discard the value.
			return ""
		}
		return string(bytes)
	}
}

// Raw returns the value as a json.RawMessage. If the value was originally created from a
// RawMessage, it returns the same value. If it is a null, it returns nil. Otherwise, it
// creates a json.RawMessage containing the JSON representation of the value.
func (v Value) Raw() json.RawMessage {
	switch v.valueType {
	case NullType:
		return nil
	case RawType:
		if o, ok := v.valueInstance.(json.RawMessage); ok {
			return o
		}
		return nil // should never happen
	default:
		bytes, err := json.Marshal(v.valueInstance)
		if err != nil {
			// It shouldn't be possible for marshalling to fail, because Value should only contain
			// JSON-compatible types. However, UnsafeValueCopy and UnsafeInnerValue do allow a
			// badly behaved application to put an incompatible type into an array or map. In this
			// case we simply discard the value.
			return nil
		}
		return json.RawMessage(bytes)
	}
}

// Count returns the number of elements in an array or JSON object. For values of any other
// type, it returns zero.
func (v Value) Count() int {
	switch o := v.valueInstance.(type) {
	case []interface{}:
		return len(o)
	case []Value:
		return len(o)
	case map[string]interface{}:
		return len(o)
	}
	return 0
}

// GetByIndex gets an element of an array by index. If the value is not an array, or if the
// index is out of range, it returns Null().
func (v Value) GetByIndex(index int) Value {
	ret, _ := v.TryGetByIndex(index)
	return ret
}

// TryGetByIndex gets an element of an array by index, with a second return value of true if
// successful. If the value is not an array, or if the index is out of range, it returns (Null(), false).
func (v Value) TryGetByIndex(index int) (Value, bool) {
	if v.valueType == ArrayType {
		if a, ok := v.valueInstance.([]interface{}); ok {
			if index >= 0 && index < len(a) {
				return fromValue(a[index], false), true
			}
		}
	}
	return Null(), false
}

// Keys returns the keys of a JSON object as a slice. If the value is not an object, it returns an
// empty slice. The method copies the keys.
func (v Value) Keys() []string {
	if v.valueType == ObjectType {
		if m, ok := v.valueInstance.(map[string]interface{}); ok {
			ret := make([]string, len(m))
			i := 0
			for key := range m {
				ret[i] = key
				i++
			}
			return ret
		}
	}
	return nil
}

// GetByKey gets a value from a JSON object by key. If the value is not an object, or if the
// key is not found, it returns Null().
func (v Value) GetByKey(name string) Value {
	ret, _ := v.TryGetByKey(name)
	return ret
}

// TryGetByKey gets a value from a JSON object by key, with a second return value of true if
// successful. If the value is not an object, or if the key is not found, it returns (Null(), false).
func (v Value) TryGetByKey(name string) (Value, bool) {
	if v.valueType == ObjectType {
		if m, ok := v.valueInstance.(map[string]interface{}); ok {
			if innerValue, ok := m[name]; ok {
				return fromValue(innerValue, false), true
			}
		}
	}
	return Null(), false
}

// MarshalJSON converts the Value to its JSON representation.
func (v Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.valueInstance)
}

// UnmarshalJSON parses a Value from JSON.
//
// Currently this method is slightly less efficient than parsing the equivalent JSON data type, so
// use it with caution in structs that will be parsed very often.
func (v *Value) UnmarshalJSON(data []byte) error {
	// This implementation is inefficient because Go's JSON parser does not support unmarshalling
	// a single arbitrary value that is not enclosed in an array or an object-- so we are enclosing
	// it in an array with [ ]. Some other parser might have better support for this. However, we
	// are not currently using Value in any of the structs that we unmarshal from JSON, so this
	// will only be used if application code puts Value into a struct and unmarshals it.
	wrappedData := make([]byte, 0, len(data)+2)
	wrappedData = append(wrappedData, []byte("[")...)
	wrappedData = append(wrappedData, data...)
	wrappedData = append(wrappedData, []byte("]")...)
	valueWrapper := make([]interface{}, 0, 1)
	err := json.Unmarshal(wrappedData, &valueWrapper)
	if err != nil {
		return err
	}
	if len(valueWrapper) != 1 {
		return errors.New("unexpected JSON parsing error")
	}
	*v = fromValue(valueWrapper[0], false)
	return nil
}
