// Copyright 2016 Attic Labs, Inc. All rights reserved.
// Licensed under the Apache License, version 2.0:
// http://www.apache.org/licenses/LICENSE-2.0

package types

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/attic-labs/noms/go/d"
	"github.com/attic-labs/noms/go/hash"
)

var EmptyStructType = MakeStructType2("")
var EmptyStruct = Struct{ValueSlice{}, EmptyStructType, &hash.Hash{}}

type StructData map[string]Value

type Struct struct {
	values []Value
	t      *Type
	h      *hash.Hash
}

func NewStruct(name string, data StructData) Struct {
	fieldNames := make(sort.StringSlice, len(data))
	i := 0
	for fn := range data {
		fieldNames[i] = fn
		i++
	}

	sort.Sort(fieldNames)
	fields := make(structFields, len(data))
	values := make(ValueSlice, len(data))
	for i, name := range fieldNames {
		fields[i] = StructField{name, data[name].Type(), false}
		values[i] = data[name]
	}

	return Struct{values, MakeStructType2(name, fields...), &hash.Hash{}}
}

func NewStructWithType(t *Type, data ValueSlice) Struct {
	desc := t.Desc.(StructDesc)
	d.PanicIfFalse(len(data) == len(desc.fields))
	for i, field := range desc.fields {
		if field.Optional {
			d.Panic("Struct values cannot have optional fields (only struct types can)")
		}
		v := data[i]
		assertSubtype(field.Type, v)
	}
	return Struct{data, t, &hash.Hash{}}
}

func (s Struct) hashPointer() *hash.Hash {
	return s.h
}

// Value interface
func (s Struct) Equals(other Value) bool {
	return s.Hash() == other.Hash()
}

func (s Struct) Less(other Value) bool {
	return valueLess(s, other)
}

func (s Struct) Hash() hash.Hash {
	if s.h.IsEmpty() {
		*s.h = getHash(s)
	}

	return *s.h
}

func (s Struct) WalkValues(cb ValueCallback) {
	for _, v := range s.values {
		cb(v)
	}
}

func (s Struct) WalkRefs(cb RefCallback) {
	for _, v := range s.values {
		v.WalkRefs(cb)
	}
}

func (s Struct) Type() *Type {
	return s.t
}

func (s Struct) desc() StructDesc {
	return s.t.Desc.(StructDesc)
}

// MaybeGet returns the value of a field in the struct. If the struct does not a have a field with
// the name name then this returns (nil, false).
func (s Struct) MaybeGet(n string) (Value, bool) {
	_, i := s.desc().findField(n)
	if i == -1 {
		return nil, false
	}
	return s.values[i], true
}

// Get returns the value of a field in the struct. If the struct does not a have a field with the
// name name then this panics.
func (s Struct) Get(n string) Value {
	_, i := s.desc().findField(n)
	if i == -1 {
		d.Chk.Fail(fmt.Sprintf(`Struct has no field "%s"`, n))
	}
	return s.values[i]
}

// Set returns a new struct where the field name has been set to value. If name is not an
// existing field in the struct or the type of value is different from the old value of the
// struct field a new struct type is created.
func (s Struct) Set(n string, v Value) Struct {
	f, i := s.desc().findField(n)
	if i == -1 || !f.Type.Equals(v.Type()) {
		// New/change field
		data := make(StructData, len(s.values)+1)
		for i, f := range s.desc().fields {
			data[f.Name] = s.values[i]
		}
		data[n] = v
		return NewStruct(s.desc().Name, data)
	}

	values := make([]Value, len(s.values))
	copy(values, s.values)
	values[i] = v
	return Struct{values, s.t, &hash.Hash{}}
}

// Delete returns a new struct where the field name has been removed.
// If name is not an existing field in the struct then the current struct is returned.
func (s Struct) Delete(n string) Struct {
	desc := s.desc()
	_, idx := desc.findField(n)
	if idx == -1 {
		return s
	}

	values := make([]Value, len(s.values)-1)
	fields := make(structFields, len(s.values)-1)
	j := 0
	for i, v := range s.values {
		if i != idx {
			values[j] = v
			fields[j] = StructField{desc.fields[i].Name, desc.fields[i].Type, false}
			j++
		}
	}

	newType := MakeStructType2(s.desc().Name, fields...)
	return NewStructWithType(newType, values)
}

func (s Struct) Diff(last Struct, changes chan<- ValueChanged, closeChan <-chan struct{}) {
	if s.Equals(last) {
		return
	}
	fs1, fs2 := s.Type().Desc.(StructDesc).fields, last.Type().Desc.(StructDesc).fields
	i1, i2 := 0, 0
	for i1 < len(fs1) && i2 < len(fs2) {
		f1, f2 := fs1[i1], fs2[i2]
		fn1, fn2 := f1.Name, f2.Name

		var change ValueChanged
		if fn1 == fn2 {
			if !s.values[i1].Equals(last.values[i2]) {
				change = ValueChanged{ChangeType: DiffChangeModified, V: String(fn1)}
			}
			i1++
			i2++
		} else if fn1 < fn2 {
			change = ValueChanged{ChangeType: DiffChangeAdded, V: String(fn1)}
			i1++
		} else {
			change = ValueChanged{ChangeType: DiffChangeRemoved, V: String(fn2)}
			i2++
		}

		if change != (ValueChanged{}) && !sendChange(changes, closeChan, change) {
			return
		}
	}

	for ; i1 < len(fs1); i1++ {
		if !sendChange(changes, closeChan, ValueChanged{ChangeType: DiffChangeAdded, V: String(fs1[i1].Name)}) {
			return
		}
	}

	for ; i2 < len(fs2); i2++ {
		if !sendChange(changes, closeChan, ValueChanged{ChangeType: DiffChangeRemoved, V: String(fs2[i2].Name)}) {
			return
		}
	}
}

var escapeChar = "Q"
var headFieldNamePattern = regexp.MustCompile("[a-zA-Z]")
var tailFieldNamePattern = regexp.MustCompile("[a-zA-Z0-9_]")
var spaceRegex = regexp.MustCompile("[ ]")
var escapeRegex = regexp.MustCompile(escapeChar)

var fieldNameComponentRe = regexp.MustCompile("^" + headFieldNamePattern.String() + tailFieldNamePattern.String() + "*")
var fieldNameRe = regexp.MustCompile(fieldNameComponentRe.String() + "$")

type encodingFunc func(string, *regexp.Regexp) string

func CamelCaseFieldName(input string) string {
	//strip invalid struct characters and leave spaces
	encode := func(s1 string, p *regexp.Regexp) string {
		if p.MatchString(s1) || spaceRegex.MatchString(s1) {
			return s1
		}
		return ""
	}

	strippedField := escapeField(input, encode)
	splitField := strings.Fields(strippedField)

	if len(splitField) == 0 {
		return ""
	}

	//Camelcase field
	output := strings.ToLower(splitField[0])
	if len(splitField) > 1 {
		for _, field := range splitField[1:] {
			output += strings.Title(strings.ToLower(field))
		}
	}
	//Because we are removing characters, we may generate an invalid field name
	//i.e. -- 1A B, we will remove the first bad chars and process until 1aB
	//1aB is invalid struct field name so we will return ""
	if !IsValidStructFieldName(output) {
		return ""
	}
	return output
}

func escapeField(input string, encode encodingFunc) string {
	output := ""
	pattern := headFieldNamePattern
	for _, ch := range input {
		output += encode(string([]rune{ch}), pattern)
		pattern = tailFieldNamePattern
	}
	return output
}

// EscapeStructField escapes names for use as noms structs with regards to non CSV imported data.
// Disallowed characters are encoded as 'Q<hex-encoded-utf8-bytes>'.
// Note that Q itself is also escaped since it is the escape character.
func EscapeStructField(input string) string {
	if !escapeRegex.MatchString(input) && IsValidStructFieldName(input) {
		return input
	}
	encode := func(s1 string, p *regexp.Regexp) string {
		if p.MatchString(s1) && s1 != escapeChar {
			return s1
		}

		var hs = fmt.Sprintf("%X", s1)
		var buf bytes.Buffer
		buf.WriteString(escapeChar)
		if len(hs) == 1 {
			buf.WriteString("0")
		}
		buf.WriteString(hs)
		return buf.String()
	}
	return escapeField(input, encode)
}

// IsValidStructFieldName returns whether the name is valid as a field name in a struct.
// Valid names must start with `a-zA-Z` and after that `a-zA-Z0-9_`.
func IsValidStructFieldName(name string) bool {
	return fieldNameRe.MatchString(name)
}

func verifyFields(fs structFields) {
	for i, f := range fs {
		verifyFieldName(f.Name)
		if i > 0 && strings.Compare(fs[i-1].Name, f.Name) >= 0 {
			d.Chk.Fail("Field names must be unique and ordered alphabetically")
		}
	}
}

func verifyName(name, kind string) {
	if !IsValidStructFieldName(name) {
		d.Panic(`Invalid struct%s name: "%s"`, kind, name)
	}
}

func verifyFieldName(name string) {
	verifyName(name, " field")
}

func verifyStructName(name string) {
	if name != "" {
		verifyName(name, "")
	}
}
