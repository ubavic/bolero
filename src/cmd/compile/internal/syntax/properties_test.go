// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import (
	"strings"
	"testing"
)

func TestPropertyTokensOnlyInBoleroMode(t *testing.T) {
	var s scanner
	s.init(strings.NewReader("@"), func(line, col uint, msg string) {}, 0)
	s.next()
	if s.tok != _EOF && s.tok != _Literal && s.tok != _Name {
		// '@' is not a token in Go mode; the scanner reports an error and skips it.
		if s.tok == _At {
			t.Fatal("@ was recognized outside Bolero mode")
		}
	}

	s.init(strings.NewReader("T@Name"), func(line, col uint, msg string) {
		t.Errorf("bolero mode: %d:%d: %s", line, col, msg)
	}, boleroTok)
	var got []token
	for s.next(); s.tok != _EOF && s.tok != _Semi; s.next() {
		got = append(got, s.tok)
	}
	want := []token{_Name, _At, _Name}
	if len(got) != len(want) {
		t.Fatalf("tokens %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens %v, want %v", got, want)
		}
	}
}

func TestPropertySyntax(t *testing.T) {
	const src = `package p
type Person struct {
	Name string { Required, MaxLength: 50 }
	Age uint
} { Table: "people" }
type Status enum {
	Active { Label: "Active" }
	Archived
} { Label: "Status" }
const _ = Person::Name@MaxLength
`
	f, err := Parse(NewFileBase("p.bo"), strings.NewReader(src), nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	td := f.DeclList[0].(*TypeDecl)
	st := td.Type.(*StructType)
	if st.FieldList[0].Props == nil || len(st.FieldList[0].Props.List) != 2 {
		t.Fatalf("field properties: %+v", st.FieldList[0].Props)
	}
	if st.FieldList[0].Props.List[0].Value != nil {
		t.Fatal("bare Required should have a nil value")
	}
	if td.Props == nil || td.Props.List[0].Name.Value != "Table" {
		t.Fatalf("type properties: %+v", td.Props)
	}
	en := f.DeclList[1].(*TypeDecl).Type.(*EnumType)
	if en.Props[0] == nil || en.Props[1] != nil {
		t.Fatalf("enum value properties: %#v", en.Props)
	}
	if f.DeclList[1].(*TypeDecl).Props == nil {
		t.Fatal("missing enum type properties")
	}
	if _, ok := f.DeclList[2].(*ConstDecl).Values.(*PropertyExpr); !ok {
		t.Fatalf("const value is %T", f.DeclList[2].(*ConstDecl).Values)
	}
}

func TestPropertySyntaxErrors(t *testing.T) {
	for _, src := range []string{
		"package p; type T struct { A, B int { X: 1 } }",
		"package p; type T struct { E { X: 1 } }",
		"package p; type T struct { A int `json:\"a\"` }",
	} {
		_, err := Parse(NewFileBase("p.bo"), strings.NewReader(src), nil, nil, 0)
		if err == nil {
			t.Errorf("parsed %s", src)
		}
	}
	// The same field syntax without a property block is ordinary Go, and
	// '@' is not a token in a Go file.
	if _, err := Parse(NewFileBase("p.go"), strings.NewReader("package p; type T struct { A, B int }"), nil, nil, 0); err != nil {
		t.Errorf("plain struct: %v", err)
	}
}
