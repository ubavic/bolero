// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// boleroEnumValue returns x if it is one of the n values of the enum type
// named typ, and panics otherwise. The compiler calls it to convert
// non-constant integers to enum types; signed reports whether x was
// converted from a signed integer.
func boleroEnumValue(x, n uint64, signed bool, typ string) uint64 {
	if x >= n {
		panic(enumError{x: int64(x), signed: signed, typ: typ})
	}
	return x
}

// An enumError reports the conversion to an enum type of an integer that
// is not one of the enum's values.
type enumError struct {
	x      int64
	signed bool
	typ    string
}

func (e enumError) RuntimeError() {}

func (e enumError) Error() string {
	b := []byte("runtime error: ")
	b = appendIntStr(b, e.x, e.signed)
	b = append(b, " is not a value of enum "...)
	b = append(b, e.typ...)
	return string(b)
}
