// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestPropertySyntax(t *testing.T) {
	const src = `package p
type Person struct {
	Name string { Required, MaxLength: 50 }
} { Table: "people" }
const _ = Person::Name@MaxLength
`
	f, err := ParseFile(token.NewFileSet(), "p.bo", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	spec := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	st := spec.Type.(*ast.StructType)
	if st.Fields.List[0].Props == nil || len(st.Fields.List[0].Props.List) != 2 {
		t.Fatalf("field properties: %+v", st.Fields.List[0].Props)
	}
	if spec.Props == nil {
		t.Fatal("missing type properties")
	}
	val := f.Decls[1].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0]
	if _, ok := val.(*ast.PropertyExpr); !ok {
		t.Fatalf("value is %T", val)
	}
}

func TestPropertySyntaxErrors(t *testing.T) {
	fset := token.NewFileSet()
	for _, src := range []string{
		"package p; type T struct { A, B int { X: 1 } }",
		"package p; type T struct { E { X: 1 } }",
		"package p; type T struct { A int `json:\"a\"` }",
	} {
		if _, err := ParseFile(fset, "p.bo", src, 0); err == nil {
			t.Errorf("parsed %s", src)
		}
	}
}
