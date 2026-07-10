// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"go/token"
	"testing"
)

func TestEnumSyntaxOnlyInBoleroFiles(t *testing.T) {
	const enumSrc = "package p; type Color enum { Red; Green }; var _ = Color::Red"
	const goSrc = "package p; var enum int; var _ = enum"

	for _, test := range []struct {
		filename string
		mode     Mode
		src      string
		ok       bool
	}{
		{"x.bo", 0, enumSrc, true},
		{"x.go", Bolero, enumSrc, true},
		{"x.go", 0, enumSrc, false},
		{"x.go", 0, goSrc, true},
	} {
		_, err := ParseFile(token.NewFileSet(), test.filename, test.src, test.mode)
		if ok := err == nil; ok != test.ok {
			t.Errorf("%s (mode %d): %q: got error %v, want ok = %v", test.filename, test.mode, test.src, err, test.ok)
		}
	}
}
