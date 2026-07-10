// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package token

import "testing"

func TestEnumKeywordOnlyInBolero(t *testing.T) {
	if got := Lookup("enum"); got != IDENT {
		t.Errorf("Lookup(%q) = %v, want IDENT", "enum", got)
	}
	if got := LookupBolero("enum"); got != ENUM {
		t.Errorf("LookupBolero(%q) = %v, want ENUM", "enum", got)
	}
	if got := LookupBolero("func"); got != FUNC {
		t.Errorf("LookupBolero(%q) = %v, want FUNC", "func", got)
	}
	if IsKeyword("enum") || !IsIdentifier("enum") {
		t.Errorf("enum must be an identifier in Go source")
	}
}
