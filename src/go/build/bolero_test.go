// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"slices"
	"strings"
	"testing"
)

func TestBoleroFiles(t *testing.T) {
	p, err := ImportDir("testdata/bolero", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, list := range []struct {
		name      string
		got, want []string
	}{
		{"GoFiles", p.GoFiles, []string{"a.go", "b.bo"}},
		{"TestGoFiles", p.TestGoFiles, []string{"b_test.bo"}},
		{"XTestGoFiles", p.XTestGoFiles, []string{"x_test.go"}},
		{"IgnoredGoFiles", p.IgnoredGoFiles, []string{"ignored.bo"}},
		{"Imports", p.Imports, []string{"fmt", "strings"}},
		{"TestImports", p.TestImports, []string{"testing"}},
	} {
		if !slices.Equal(list.got, list.want) {
			t.Errorf("%s = %q, want %q", list.name, list.got, list.want)
		}
	}
}

func TestBoleroCgo(t *testing.T) {
	_, err := ImportDir("testdata/bolero_cgo", 0)
	if err == nil || !strings.Contains(err.Error(), "use of cgo in Bolero file") {
		t.Fatalf("ImportDir: got error %v, want use of cgo in Bolero file", err)
	}
}
