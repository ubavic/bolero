// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package load

import (
	"internal/bolero"

	"cmd/go/internal/str"
)

// addMetaDep appends bolero/meta to imports when files contains a Bolero
// source file and the package is not bolero/meta itself. Every such package
// depends on bolero/meta so the compiler can register static properties.
func addMetaDep(self string, files, imports []string) []string {
	if self == "bolero/meta" {
		return imports
	}
	has := false
	for _, f := range files {
		if bolero.IsSourceFile(f) {
			has = true
			break
		}
	}
	if !has {
		return imports
	}
	for _, path := range imports {
		if path == "bolero/meta" {
			return imports
		}
	}
	out := make([]string, len(imports)+1)
	copy(out, imports)
	out[len(imports)] = "bolero/meta"
	return out
}

// testmainFile returns the name of the generated main file of the test
// binary for p. Since the test main uses the tests, it is a Bolero source
// file if any test file of p is: Go source files cannot use declarations
// from Bolero source files.
func testmainFile(p *Package) string {
	for _, f := range str.StringList(p.TestGoFiles, p.XTestGoFiles) {
		if bolero.IsSourceFile(f) {
			return "_testmain" + bolero.FileExt
		}
	}
	return "_testmain.go"
}
