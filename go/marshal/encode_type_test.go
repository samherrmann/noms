// Copyright 2017 Attic Labs, Inc. All rights reserved.
// Licensed under the Apache License, version 2.0:
// http://www.apache.org/licenses/LICENSE-2.0

package marshal

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/attic-labs/noms/go/types"
	"github.com/attic-labs/testify/assert"
)

func TestMarshalTypeType(tt *testing.T) {
	t := func(exp *types.Type, ptr interface{}) {
		p := reflect.ValueOf(ptr)
		assert.NotEqual(tt, reflect.Ptr, p.Type().Kind())
		actual, err := MarshalType(p.Interface())
		assert.NoError(tt, err)
		assert.NotNil(tt, actual, "%#v", p.Interface())
		assert.True(tt, exp.Equals(actual))
	}

	t(types.NumberType, float32(0))
	t(types.NumberType, float64(0))
	t(types.NumberType, int(0))
	t(types.NumberType, int16(0))
	t(types.NumberType, int32(0))
	t(types.NumberType, int64(0))
	t(types.NumberType, int8(0))
	t(types.NumberType, uint(0))
	t(types.NumberType, uint16(0))
	t(types.NumberType, uint32(0))
	t(types.NumberType, uint64(0))
	t(types.NumberType, uint8(0))

	t(types.BoolType, true)
	t(types.StringType, "hi")

	var l []int
	t(types.MakeListType(types.NumberType), l)

	var m map[uint32]string
	t(types.MakeMapType(types.NumberType, types.StringType), m)

	type TestStruct struct {
		Str string
		Num float64
	}
	var str TestStruct
	t(types.MakeStructTypeFromFields("TestStruct", types.FieldMap{
		"str": types.StringType,
		"num": types.NumberType,
	}), str)

	// Same again to test caching
	t(types.MakeStructTypeFromFields("TestStruct", types.FieldMap{
		"str": types.StringType,
		"num": types.NumberType,
	}), str)

	anonStruct := struct {
		B bool
	}{
		true,
	}
	t(types.MakeStructTypeFromFields("", types.FieldMap{
		"b": types.BoolType,
	}), anonStruct)

	type TestNestedStruct struct {
		A []int16
		B TestStruct
		C float64
	}
	var nestedStruct TestNestedStruct
	t(types.MakeStructTypeFromFields("TestNestedStruct", types.FieldMap{
		"a": types.MakeListType(types.NumberType),
		"b": types.MakeStructTypeFromFields("TestStruct", types.FieldMap{
			"str": types.StringType,
			"num": types.NumberType,
		}),
		"c": types.NumberType,
	}), nestedStruct)

	type testStruct struct {
		Str string
		Num float64
	}
	var ts testStruct
	t(types.MakeStructTypeFromFields("TestStruct", types.FieldMap{
		"str": types.StringType,
		"num": types.NumberType,
	}), ts)
}

//
func assertMarshalTypeErrorMessage(t *testing.T, v interface{}, expectedMessage string) {
	_, err := MarshalType(v)
	assert.Error(t, err)
	assert.Equal(t, expectedMessage, err.Error())
}

func TestMarshalTypeInvalidTypes(t *testing.T) {
	assertMarshalTypeErrorMessage(t, make(chan int), "Type is not supported, type: chan int")
	l := types.NewList()
	assertMarshalTypeErrorMessage(t, l, "Cannot marshal type types.List, it requires type parameters")
}

func TestMarshalTypeEmbeddedStruct(t *testing.T) {
	type EmbeddedStruct struct{}
	type TestStruct struct {
		EmbeddedStruct
	}
	assertMarshalTypeErrorMessage(t, TestStruct{EmbeddedStruct{}}, "Embedded structs are not supported, type: marshal.TestStruct")
}

func TestMarshalTypeEncodeNonExportedField(t *testing.T) {
	type TestStruct struct {
		x int
	}
	assertMarshalTypeErrorMessage(t, TestStruct{1}, "Non exported fields are not supported, type: marshal.TestStruct")
}

func TestMarshalTypeEncodeNomsTypeWithTypeParameters(t *testing.T) {

	assertMarshalTypeErrorMessage(t, types.NewList(), "Cannot marshal type types.List, it requires type parameters")
	assertMarshalTypeErrorMessage(t, types.NewSet(), "Cannot marshal type types.Set, it requires type parameters")
	assertMarshalTypeErrorMessage(t, types.NewMap(), "Cannot marshal type types.Map, it requires type parameters")
	assertMarshalTypeErrorMessage(t, types.NewRef(types.NewSet()), "Cannot marshal type types.Ref, it requires type parameters")
}

func TestMarshalTypeEncodeTaggingSkip(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		Abc int `noms:"-"`
		Def bool
	}
	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructTypeFromFields("S", types.FieldMap{
		"def": types.BoolType,
	}).Equals(typ))
}

func TestMarshalTypeNamedFields(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		Aaa int  `noms:"a"`
		Bbb bool `noms:"B"`
		Ccc string
	}
	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructTypeFromFields("S", types.FieldMap{
		"a":   types.NumberType,
		"B":   types.BoolType,
		"ccc": types.StringType,
	}).Equals(typ))
}

func TestMarshalTypeInvalidNamedFields(t *testing.T) {
	type S struct {
		A int `noms:"1a"`
	}
	var s S
	assertMarshalTypeErrorMessage(t, s, "Invalid struct field name: 1a")
}

func TestMarshalTypeOmitEmpty(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		String string `noms:",omitempty"`
	}
	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructType2("S", types.StructField{"string", types.StringType, true}).Equals(typ))
}

func ExampleMarshalType() {
	type Person struct {
		Given  string
		Female bool
	}
	var person Person
	personNomsType, err := MarshalType(person)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(personNomsType.Describe())
	// Output: struct Person {
	//   female: Bool,
	//   given: String,
	// }
}

func TestMarshalTypeSlice(t *testing.T) {
	assert := assert.New(t)
	s := []string{"a", "b", "c"}
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeListType(types.StringType).Equals(typ))
}

func TestMarshalTypeArray(t *testing.T) {
	assert := assert.New(t)
	a := [3]int{1, 2, 3}
	typ, err := MarshalType(a)
	assert.NoError(err)
	assert.True(types.MakeListType(types.NumberType).Equals(typ))
}

func TestMarshalTypeStructWithSlice(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		List []int
	}
	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructTypeFromFields("S", types.FieldMap{
		"list": types.MakeListType(types.NumberType),
	}).Equals(typ))
}

func TestMarshalTypeRecursive(t *testing.T) {
	assert := assert.New(t)

	type Node struct {
		Value    int
		Children []Node
	}
	var n Node
	typ, err := MarshalType(n)
	assert.NoError(err)

	typ2 := types.MakeStructType2("Node",
		types.StructField{
			Name: "children",
			Type: types.MakeListType(types.MakeCycleType(0)),
		},
		types.StructField{
			Name: "value",
			Type: types.NumberType,
		},
	)
	assert.True(typ2.Equals(typ))
}

func TestMarshalTypeMap(t *testing.T) {
	assert := assert.New(t)

	var m map[string]int
	typ, err := MarshalType(m)
	assert.NoError(err)
	assert.True(types.MakeMapType(types.StringType, types.NumberType).Equals(typ))

	type S struct {
		N string
	}

	var m2 map[S]bool
	typ, err = MarshalType(m2)
	assert.NoError(err)
	assert.True(types.MakeMapType(
		types.MakeStructTypeFromFields("S", types.FieldMap{
			"n": types.StringType,
		}),
		types.BoolType).Equals(typ))
}

func TestMarshalTypeSet(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		A map[int]struct{} `noms:",set"`
		B map[int]struct{}
		C map[int]string      `noms:",set"`
		D map[string]struct{} `noms:",set"`
		E map[string]struct{}
		F map[string]int `noms:",set"`
		G []int          `noms:",set"`
		H string         `noms:",set"`
	}
	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)

	emptyStructType := types.MakeStructTypeFromFields("", types.FieldMap{})

	assert.True(types.MakeStructTypeFromFields("S", types.FieldMap{
		"a": types.MakeSetType(types.NumberType),
		"b": types.MakeMapType(types.NumberType, emptyStructType),
		"c": types.MakeMapType(types.NumberType, types.StringType),
		"d": types.MakeSetType(types.StringType),
		"e": types.MakeMapType(types.StringType, emptyStructType),
		"f": types.MakeMapType(types.StringType, types.NumberType),
		"g": types.MakeListType(types.NumberType),
		"h": types.StringType,
	}).Equals(typ))
}

func TestMarshalTypeSetWithTags(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		A map[int]struct{} `noms:"foo,set"`
		B map[int]struct{} `noms:",omitempty,set"`
		C map[int]struct{} `noms:"bar,omitempty,set"`
	}

	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructType2("S",
		types.StructField{"foo", types.MakeSetType(types.NumberType), false},
		types.StructField{"b", types.MakeSetType(types.NumberType), true},
		types.StructField{"bar", types.MakeSetType(types.NumberType), true},
	).Equals(typ))
}

func TestMarshalTypeInvalidTag(t *testing.T) {
	type S struct {
		F string `noms:",omitEmpty"`
	}
	var s S
	_, err := MarshalType(s)
	assert.Error(t, err)
	assert.Equal(t, `Unrecognized tag: omitEmpty`, err.Error())
}

func TestMarshalTypeCanSkipUnexportedField(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		Abc         int
		notExported bool `noms:"-"`
	}
	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructTypeFromFields("S", types.FieldMap{
		"abc": types.NumberType,
	}).Equals(typ))
}

func TestMarshalTypeOriginal(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		Foo int          `noms:",omitempty"`
		Bar types.Struct `noms:",original"`
	}

	var s S
	typ, err := MarshalType(s)
	assert.NoError(err)
	assert.True(types.MakeStructType2("S",
		types.StructField{"foo", types.NumberType, true},
	).Equals(typ))
}

func TestMarshalTypeNomsTypes(t *testing.T) {
	assert := assert.New(t)

	type S struct {
		Blob   types.Blob
		Bool   types.Bool
		Number types.Number
		String types.String
		Type   *types.Type
	}
	var s S
	assert.True(MustMarshalType(s).Equals(
		types.MakeStructTypeFromFields("S", types.FieldMap{
			"blob":   types.BlobType,
			"bool":   types.BoolType,
			"number": types.NumberType,
			"string": types.StringType,
			"type":   types.TypeType,
		}),
	))
}

func (t primitiveType) MarshalNomsType() (*types.Type, error) {
	return types.NumberType, nil
}

func TestTypeMarshalerPrimitiveType(t *testing.T) {
	assert := assert.New(t)

	var u primitiveType
	typ := MustMarshalType(u)
	assert.Equal(types.NumberType, typ)
}

func (u primitiveSliceType) MarshalNomsType() (*types.Type, error) {
	return types.StringType, nil
}

func TestTypeMarshalerPrimitiveSliceType(t *testing.T) {
	assert := assert.New(t)

	var u primitiveSliceType
	typ := MustMarshalType(u)
	assert.Equal(types.StringType, typ)
}

func (u primitiveMapType) MarshalNomsType() (*types.Type, error) {
	return types.MakeSetType(types.StringType), nil
}

func TestTypeMarshalerPrimitiveMapType(t *testing.T) {
	assert := assert.New(t)

	var u primitiveMapType
	typ := MustMarshalType(u)
	assert.Equal(types.MakeSetType(types.StringType), typ)
}

func TestTypeMarshalerPrimitiveStructTypeNoMarshalNomsType(t *testing.T) {
	assert := assert.New(t)

	var u primitiveStructType
	_, err := MarshalType(u)
	assert.Error(err)
	assert.Equal("Cannot marshal type which implements marshal.Marshaler, perhaps implement marshal.TypeMarshaler for marshal.primitiveStructType", err.Error())
}

func (u builtinType) MarshalNomsType() (*types.Type, error) {
	return types.StringType, nil
}

func TestTypeMarshalerBuiltinType(t *testing.T) {
	assert := assert.New(t)

	var u builtinType
	typ := MustMarshalType(u)
	assert.Equal(types.StringType, typ)
}

func (u wrappedMarshalerType) MarshalNomsType() (*types.Type, error) {
	return types.NumberType, nil
}

func TestTypeMarshalerWrapperMarshalerType(t *testing.T) {
	assert := assert.New(t)

	var u wrappedMarshalerType
	typ := MustMarshalType(u)
	assert.Equal(types.NumberType, typ)
}

func (u returnsMarshalerError) MarshalNomsType() (*types.Type, error) {
	return nil, errors.New("expected error")
}

func (u returnsMarshalerNil) MarshalNomsType() (*types.Type, error) {
	return nil, nil
}

func (u panicsMarshaler) MarshalNomsType() (*types.Type, error) {
	panic("panic")
}

func TestTypeMarshalerErrors(t *testing.T) {
	assert := assert.New(t)

	expErr := errors.New("expected error")
	var m1 returnsMarshalerError
	_, actErr := MarshalType(m1)
	assert.Equal(expErr, actErr)

	var m2 returnsMarshalerNil
	assert.Panics(func() { MarshalType(m2) })

	var m3 panicsMarshaler
	assert.Panics(func() { MarshalType(m3) })
}
