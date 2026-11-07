// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bolero

import "testing"

func TestFileNames(t *testing.T) {
	for _, test := range []struct {
		name               string
		source, goOrBolero bool
		test               bool
		stem, ext          string
	}{
		{"x.bo", true, true, false, "x", ".bo"},
		{"dir/x_test.bo", true, true, true, "dir/x_test", ".bo"},
		{"x.go", false, true, false, "x", ".go"},
		{"x_test.go", false, true, true, "x_test", ".go"},
		{"x.bo.go", false, true, false, "x.bo", ".go"},
		{"x.boo", false, false, false, "x.boo", ""},
		{"x.c", false, false, false, "x.c", ""},
		{"", false, false, false, "", ""},
	} {
		if got := IsSourceFile(test.name); got != test.source {
			t.Errorf("IsSourceFile(%q) = %v, want %v", test.name, got, test.source)
		}
		if got := IsGoOrBoleroFile(test.name); got != test.goOrBolero {
			t.Errorf("IsGoOrBoleroFile(%q) = %v, want %v", test.name, got, test.goOrBolero)
		}
		if got := IsTestFile(test.name); got != test.test {
			t.Errorf("IsTestFile(%q) = %v, want %v", test.name, got, test.test)
		}
		if stem, ext := TrimExt(test.name); stem != test.stem || ext != test.ext {
			t.Errorf("TrimExt(%q) = %q, %q, want %q, %q", test.name, stem, ext, test.stem, test.ext)
		}
	}
}
