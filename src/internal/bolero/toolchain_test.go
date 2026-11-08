// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bolero

import "testing"

func TestExecName(t *testing.T) {
	for _, test := range []struct {
		importPath string
		name       string
		ok         bool
	}{
		{"cmd/go", "bolero", true},
		{"cmd/gofmt", "bolerofmt", true},
		{"cmd/compile", "", false},
		{"cmd/go/internal/base", "", false},
		{"example.com/cmd/go", "", false},
		{"", "", false},
	} {
		name, ok := ExecName(test.importPath)
		if name != test.name || ok != test.ok {
			t.Errorf("ExecName(%q) = %q, %v, want %q, %v", test.importPath, name, ok, test.name, test.ok)
		}
	}
}
