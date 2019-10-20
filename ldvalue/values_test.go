package ldvalue

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNullValue(t *testing.T) {
	v := Null()
	assert.Equal(t, NullType, v.Type())
	assert.Nil(t, v.InnerValue())
	assert.True(t, v.IsNull())
	assert.False(t, v.IsNumber())
	assert.False(t, v.IsInt())
	assert.Equal(t, v, Null())
	assert.Equal(t, v, Value{})
}

func TestBoolValue(t *testing.T) {
	tv := Bool(true)
	assert.Equal(t, BoolType, tv.Type())
	assert.True(t, tv.Bool())
	assert.Equal(t, true, tv.InnerValue())
	assert.False(t, tv.IsNull())
	assert.False(t, tv.IsNumber())
	assert.False(t, tv.IsInt())
	assert.Equal(t, tv, Bool(true))

	fv := Bool(false)
	assert.Equal(t, BoolType, fv.Type())
	assert.False(t, fv.Bool())
	assert.Equal(t, false, fv.InnerValue())
	assert.False(t, fv.IsNull())
	assert.False(t, fv.IsNumber())
	assert.False(t, fv.IsInt())
	assert.Equal(t, fv, Bool(false))
}

func TestBoolIsFalseForNonBooleans(t *testing.T) {
	assert.False(t, Null().Bool())
	assert.False(t, Int(0).Bool())
	assert.False(t, Float64(0).Bool())
	assert.False(t, String("").Bool())
}

func TestIntValue(t *testing.T) {
	v := Int(2)
	assert.Equal(t, NumberType, v.Type())
	assert.Equal(t, 2, v.Int())
	assert.Equal(t, float64(2), v.Float64())
	assert.False(t, v.IsNull())
	assert.True(t, v.IsNumber())
	assert.True(t, v.IsInt())
}

func TestIntIsZeroForNonNumbers(t *testing.T) {
	assert.Equal(t, 0, Null().Int())
	assert.Equal(t, 0, Bool(true).Int())
	assert.Equal(t, 0, String("1").Int())
}

func TestFloat64Value(t *testing.T) {
	v := Float64(2.75)
	assert.Equal(t, NumberType, v.Type())
	assert.Equal(t, 2, v.Int())
	assert.Equal(t, 2.75, v.Float64())
	assert.False(t, v.IsNull())
	assert.True(t, v.IsNumber())
	assert.False(t, v.IsInt())

	floatButReallyInt := Float64(2.0)
	assert.Equal(t, NumberType, floatButReallyInt.Type())
	assert.Equal(t, 2, floatButReallyInt.Int())
	assert.Equal(t, 2.0, floatButReallyInt.Float64())
	assert.False(t, floatButReallyInt.IsNull())
	assert.True(t, floatButReallyInt.IsNumber())
	assert.True(t, floatButReallyInt.IsInt())
}

func TestFloat64IsZeroForNonNumbers(t *testing.T) {
	assert.Equal(t, float64(0), Null().Float64())
	assert.Equal(t, float64(0), Bool(true).Float64())
	assert.Equal(t, float64(0), String("1").Float64())
}

func TestStringValue(t *testing.T) {
	v := String("abc")
	assert.Equal(t, StringType, v.Type())
	assert.Equal(t, "abc", v.String())
	assert.False(t, v.IsNull())
	assert.False(t, v.IsNumber())
	assert.False(t, v.IsInt())
	assert.Equal(t, v, String("abc"))
}

func TestStringIsEmptyForNonStrings(t *testing.T) {
	assert.Equal(t, "", Null().String())
	assert.Equal(t, "", Bool(true).String())
	assert.Equal(t, "", Float64(0).String())
	assert.Equal(t, "", ArrayCopy([]interface{}{1, 2}).String())
	assert.Equal(t, "", ObjectCopy(map[string]interface{}{"a": 1}).String())
}

func TestJSONString(t *testing.T) {
	assert.Equal(t, "null", Null().JSONString())
	assert.Equal(t, "false", Bool(false).JSONString())
	assert.Equal(t, "true", Bool(true).JSONString())
	assert.Equal(t, "1", Int(1).JSONString())
	assert.Equal(t, "1", Float64(1.0).JSONString())
	assert.Equal(t, "1.5", Float64(1.5).JSONString())
	assert.Equal(t, `"\"hi\"\r"`, String("\"hi\"\r").JSONString())
	assert.Equal(t, "[1,2]", ArrayCopy([]interface{}{1, 2}).JSONString())
	assert.Equal(t, `{"a":1}`, ObjectCopy(map[string]interface{}{"a": 1}).JSONString())
}

func TestArrayCopy(t *testing.T) {
	numValue := 1
	mapShouldBe := map[string]interface{}{"a": "b"}
	arrayShouldBe := []interface{}{numValue, mapShouldBe}
	mutableMap := map[string]interface{}{"a": "b"}
	value := ArrayCopy([]interface{}{numValue, mutableMap})

	assert.Equal(t, ArrayType, value.Type())
	assert.Equal(t, 2, value.Count())

	assert.Equal(t, Int(numValue), value.GetByIndex(0))
	item, ok := value.TryGetByIndex(0)
	assert.True(t, ok)
	assert.Equal(t, Int(numValue), item)

	assert.Equal(t, mapShouldBe, value.GetByIndex(1).InnerValue())
	mutableMap["a"] = "different" // the value in our built array should be deep-copied, so this shouldn't affect it
	assert.Equal(t, mapShouldBe, value.GetByIndex(1).InnerValue())

	newArray := value.InnerValue().([]interface{})
	assert.Equal(t, arrayShouldBe, newArray)
	arrayShouldBe[0] = 99
	assert.NotEqual(t, arrayShouldBe[0], newArray[0])
}

func TestArrayBuild(t *testing.T) {
	firstValue := "a"
	otherValue := "other"
	b := ArrayBuild(0).
		Add(String(firstValue)).
		Add(String(otherValue))
	value := b.Build()

	assert.Equal(t, ArrayType, value.Type())
	assert.Equal(t, 2, value.Count())

	assert.Equal(t, String(firstValue), value.GetByIndex(0))
	item, ok := value.TryGetByIndex(0)
	assert.True(t, ok)
	assert.Equal(t, String(firstValue), item)

	originalArray := []interface{}{firstValue, otherValue}
	newArray := value.InnerValue().([]interface{})
	assert.Equal(t, originalArray, newArray)

	b.Add(String("more"))
	valueAfterModifyingBuilder := b.Build()
	assert.Equal(t, ArrayType, valueAfterModifyingBuilder.Type())
	modifiedArray := []interface{}{firstValue, otherValue, "more"}
	assert.Equal(t, modifiedArray, valueAfterModifyingBuilder.InnerValue())
	assert.Equal(t, originalArray, value.InnerValue())
}

func TestGetByIndexForInvalidIndex(t *testing.T) {
	value := ArrayCopy([]interface{}{1, 2})

	assert.Equal(t, Null(), value.GetByIndex(-1))
	item, ok := value.TryGetByIndex(-1)
	assert.False(t, ok)
	assert.Equal(t, Null(), item)

	assert.Equal(t, Null(), value.GetByIndex(2))
	item, ok = value.TryGetByIndex(2)
	assert.False(t, ok)
	assert.Equal(t, Null(), item)
}

func TestSimpleTypesAreTreatedAsEmptyArray(t *testing.T) {
	for _, value := range []Value{Null(), Bool(true), Int(0), Float64(0)} {
		t.Run(fmt.Sprintf("type: %s", value), func(t *testing.T) {
			assert.Equal(t, 0, value.Count())
			x, ok := value.TryGetByIndex(0)
			assert.False(t, ok)
			assert.Equal(t, NullType, x.Type())
		})
	}
}

func TestObjectCopy(t *testing.T) {
	arrayShouldBe := []interface{}{1, 2}
	mutableArray := []interface{}{1, 2}
	mapShouldBe := map[string]interface{}{"a": arrayShouldBe, "b": 3}
	mutableMap := map[string]interface{}{"a": mutableArray, "b": 3}
	value := ObjectCopy(mutableMap)

	assert.Equal(t, ObjectType, value.Type())
	assert.Equal(t, 2, value.Count())

	assert.Equal(t, Int(3), value.GetByKey("b"))
	item, ok := value.TryGetByKey("b")
	assert.True(t, ok)
	assert.Equal(t, Int(3), item)

	assert.Equal(t, arrayShouldBe, value.GetByKey("a").InnerValue())
	mutableMap["a"] = Int(4)
	mutableArray[0] = "different" // the values in our built object should be deep-copied, so this shouldn't affect it
	assert.Equal(t, mapShouldBe, value.InnerValue())
}

func TestObjectBuild(t *testing.T) {
	b := ObjectBuild(0).
		Set("a", ArrayBuild(0).Add(String("1")).Add(String("2")).Build()).
		Set("b", String("3"))
	value := b.Build()

	assert.Equal(t, ObjectType, value.Type())
	assert.Equal(t, 2, value.Count())
	keys := value.Keys()
	sort.Strings(keys)
	assert.Equal(t, []string{"a", "b"}, keys)

	assert.Equal(t, String("3"), value.GetByKey("b"))
	item, ok := value.TryGetByKey("b")
	assert.True(t, ok)
	assert.Equal(t, String("3"), item)

	originalMap := map[string]interface{}{"a": []interface{}{"1", "2"}, "b": "3"}
	assert.Equal(t, originalMap, value.InnerValue())

	b.Set("a", String("2"))
	valueAfterModifyingBuilder := b.Build()
	assert.Equal(t, map[string]interface{}{"a": "2", "b": "3"}, valueAfterModifyingBuilder.InnerValue())
	assert.Equal(t, originalMap, value.InnerValue())
}

func TestGetByKeyForInvalidKey(t *testing.T) {
	value := ObjectCopy(map[string]interface{}{"a": "1"})
	assert.Equal(t, Null(), value.GetByKey("b"))
	item, ok := value.TryGetByKey("b")
	assert.False(t, ok)
	assert.Equal(t, Null(), item)
}

func TestSimpleTypesAreTreatedAsEmptyMap(t *testing.T) {
	for _, value := range []Value{Null(), Bool(true), Int(0), Float64(0)} {
		t.Run(fmt.Sprintf("type %s, value %v", value.Type(), value.InnerValue()), func(t *testing.T) {
			assert.Equal(t, []string(nil), value.Keys())
			assert.Equal(t, Null(), value.GetByKey("name"))
			x, ok := value.TryGetByKey("name")
			assert.False(t, ok)
			assert.Equal(t, NullType, x.Type())
		})
	}
}

func TestSameIntegerValuesWithAnyNumericConstructorAreEqual(t *testing.T) {
	assert.Equal(t, Int(2), Float64(2))
	assert.Equal(t, ValueCopy(int8(2)), Float64(2))
	assert.Equal(t, ValueCopy(uint8(2)), Float64(2))
	assert.Equal(t, ValueCopy(int16(2)), Float64(2))
	assert.Equal(t, ValueCopy(uint16(2)), Float64(2))
	assert.Equal(t, ValueCopy(int32(2)), Float64(2))
	assert.Equal(t, ValueCopy(uint32(2)), Float64(2))
	assert.Equal(t, ValueCopy(float32(2)), Float64(2))
}

func TestJsonMarshalUnmarshal(t *testing.T) {
	items := []struct {
		value Value
		json  string
	}{
		{Null(), "null"},
		{Int(1), "1"},
		{Float64(1), "1"},
		{Float64(2.5), "2.5"},
		{String("x"), `"x"`},
		{ArrayCopy([]interface{}{true, "x"}), `[true,"x"]`},
		{ObjectCopy(map[string]interface{}{"a": true}), `{"a":true}`},
	}
	for _, item := range items {
		t.Run(fmt.Sprintf("type %s, json %v", item.value.Type(), item.json), func(t *testing.T) {
			j, err := json.Marshal(item.value)
			assert.NoError(t, err)
			assert.Equal(t, item.json, string(j))

			var v Value
			err = json.Unmarshal([]byte(item.json), &v)
			assert.NoError(t, err)
			assert.Equal(t, item.value, v)
		})
	}
}
