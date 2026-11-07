// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package meta reads the static properties of Bolero declarations
// and holds field descriptors.
//
// A field descriptor such as Person::Name has type Field[Person, string].
// The compiler turns it into a call to [NewField]. Properties attached to
// a struct or enum type, and to enum values, are registered from a
// generated init by [RegisterType] and [RegisterValue]. Reading a property
// with the @ operator does not use this package; it is a compile-time constant.
package meta

import (
	"reflect"
	"sync"
)

// Properties holds the static properties of one declaration.
// The zero Properties has no entries. Properties values are read-only.
type Properties struct {
	names []string
	vals  []any
}

// Names returns the property names in declaration order.
func (p Properties) Names() []string {
	if len(p.names) == 0 {
		return nil
	}
	out := make([]string, len(p.names))
	copy(out, p.names)
	return out
}

// Lookup returns the value of the property name.
// The boolean is false when name is not one of the properties.
// An untyped constant is stored as its default type: int, float64,
// complex128, string, bool, or int32 for an untyped rune.
// A typed constant keeps its type.
func (p Properties) Lookup(name string) (any, bool) {
	for i, n := range p.names {
		if n == name {
			return p.vals[i], true
		}
	}
	return nil, false
}

// NewProperties returns properties from alternating names and values.
// A bare property name in a Bolero property block is the value true.
// NewProperties is the plain-Go form of a property block; the compiler
// calls it from generated code.
func NewProperties(pairs ...any) Properties {
	if len(pairs)%2 != 0 {
		panic("meta: NewProperties called with an odd number of arguments")
	}
	p := Properties{
		names: make([]string, 0, len(pairs)/2),
		vals:  make([]any, 0, len(pairs)/2),
	}
	for i := 0; i < len(pairs); i += 2 {
		name, ok := pairs[i].(string)
		if !ok {
			panic("meta: property name is not a string")
		}
		p.names = append(p.names, name)
		p.vals = append(p.vals, pairs[i+1])
	}
	return p
}

// Field describes one field of struct type S, of type T.
// Person::Name has type Field[Person, string].
// The zero Field is comparable and not equal to any descriptor.
// Two descriptors of the same field of the same struct type are equal.
type Field[S, T any] struct {
	info *fieldInfo
}

// Name returns the field's name.
func (f Field[S, T]) Name() string {
	if f.info == nil {
		panic("meta: Name of zero Field")
	}
	return f.info.name
}

// Get returns the field's value in s.
func (f Field[S, T]) Get(s S) T {
	if f.info == nil {
		panic("meta: Get of zero Field")
	}
	return f.info.get(s).(T)
}

// Set sets the field's value in s to v.
func (f Field[S, T]) Set(s *S, v T) {
	if f.info == nil {
		panic("meta: Set of zero Field")
	}
	if s == nil {
		panic("meta: Set of nil pointer")
	}
	f.info.set(s, v)
}

// Properties returns the static properties of the field.
// A promoted field has the properties declared on the field it comes from.
func (f Field[S, T]) Properties() Properties {
	if f.info == nil {
		return Properties{}
	}
	return f.info.props
}

type fieldInfo struct {
	name  string
	props Properties
	get   func(any) any
	set   func(any, any)
}

type fieldKey struct {
	typ  reflect.Type
	name string
}

var (
	fieldMu sync.Mutex
	fields  = map[fieldKey]*fieldInfo{}
)

// NewField returns the descriptor for the named field of S.
// get and set read and write that field. Repeated calls for the same
// struct type and field name return equal descriptors.
// NewField is the plain-Go form of a field descriptor; the compiler calls it.
func NewField[S, T any](name string, props Properties, get func(S) T, set func(*S, T)) Field[S, T] {
	key := fieldKey{reflect.TypeFor[S](), name}
	fieldMu.Lock()
	defer fieldMu.Unlock()
	info := fields[key]
	if info == nil {
		info = &fieldInfo{
			name:  name,
			props: props,
			get:   func(s any) any { return get(s.(S)) },
			set:   func(s, v any) { set(s.(*S), v.(T)) },
		}
		fields[key] = info
	}
	return Field[S, T]{info}
}

var (
	regMu      sync.Mutex
	typeProps  = map[reflect.Type]Properties{}
	valueProps = map[valueKey]Properties{}
)

type valueKey struct {
	typ reflect.Type
	val int64
}

// RegisterType records the static properties of struct or enum type T.
// The compiler calls it from a generated init. A later call for the same
// T replaces the properties.
func RegisterType[T any](p Properties) {
	regMu.Lock()
	defer regMu.Unlock()
	typeProps[reflect.TypeFor[T]()] = p
}

// TypeProperties returns the static properties of struct or enum type T.
// The result is zero when T has none.
func TypeProperties[T any]() Properties {
	regMu.Lock()
	defer regMu.Unlock()
	return typeProps[reflect.TypeFor[T]()]
}

// RegisterValue records the static properties of enum value v.
// The compiler calls it from a generated init.
func RegisterValue[E ~int](v E, p Properties) {
	regMu.Lock()
	defer regMu.Unlock()
	valueProps[valueKey{reflect.TypeOf(v), int64(v)}] = p
}

// ValueProperties returns the static properties of enum value v.
// The result is zero when v has none.
func ValueProperties[E ~int](v E) Properties {
	regMu.Lock()
	defer regMu.Unlock()
	return valueProps[valueKey{reflect.TypeOf(v), int64(v)}]
}
