// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bolero holds definitions shared by the parts of the toolchain
// that treat Bolero source files differently from Go source files.
//
// Bolero syntax is accepted only in Bolero source files.
// Go source files are processed exactly as by the upstream Go toolchain.
package bolero

// FileExt is the file name extension of Bolero source files.
const FileExt = ".bo"

// AppJSON is the name of a Bolero project's manifest.
// The main module of a Bolero project uses this file instead of go.mod.
const AppJSON = "app.json"

// IsSourceFile reports whether name is the name of a Bolero source file.
func IsSourceFile(name string) bool {
	return hasSuffix(name, FileExt)
}

// IsGoOrBoleroFile reports whether name is the name of a Go or a Bolero source file.
func IsGoOrBoleroFile(name string) bool {
	return hasSuffix(name, ".go") || hasSuffix(name, FileExt)
}

// IsGoOrBoleroExt reports whether ext is the file name extension of
// Go or Bolero source files.
func IsGoOrBoleroExt(ext string) bool {
	return ext == ".go" || ext == FileExt
}

// IsTestFile reports whether name is the name of a Go or a Bolero test file.
func IsTestFile(name string) bool {
	return hasSuffix(name, "_test.go") || hasSuffix(name, "_test"+FileExt)
}

// TrimExt returns name without its Go or Bolero file name extension,
// and the extension.
func TrimExt(name string) (stem, ext string) {
	for _, ext := range [...]string{".go", FileExt} {
		if hasSuffix(name, ext) {
			return name[:len(name)-len(ext)], ext
		}
	}
	return name, ""
}

// MarkGoTypesObject, if set, marks a go/types object as declared in a
// Bolero source file. It is set by go/types and called by importers,
// which can construct go/types objects only through its exported API.
var MarkGoTypesObject func(obj any)

// SetGoTypesEnum, if set, records that a go/types type name declares an
// enum type with the given values. Like MarkGoTypesObject, it is set by
// go/types and called by importers.
var SetGoTypesEnum func(tn any, names []string)

// AddGoTypesProp, if set, records one static property on a go/types type
// name. on is 't', 'f' or 'v'. name is the field or enum value name.
// val is the constant's Go value: bool, string, int64, *big.Int, *big.Rat,
// *big.Float, or ComplexVal. typ is the go/types type of the constant.
var AddGoTypesProp func(tn any, on byte, name, key string, val, typ any)

// ComplexVal is the real and imaginary parts of a complex constant,
// each a value as accepted by AddGoTypesProp.
type ComplexVal struct{ Re, Im any }

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
