// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import (
	"slices"
	"strings"
	"testing"
)

func TestEnumTokensOnlyInBoleroMode(t *testing.T) {
	for _, test := range []struct {
		mode uint
		want []token
	}{
		{0, []token{_Name, _Colon, _Colon}},
		{boleroTok, []token{_Enum, _EnumAccess}},
	} {
		var s scanner
		errh := func(line, col uint, msg string) { t.Errorf("%d:%d: %s", line, col, msg) }
		s.init(strings.NewReader("enum ::"), errh, test.mode)
		var got []token
		for s.next(); s.tok != _EOF; s.next() {
			got = append(got, s.tok)
		}
		if !slices.Equal(got, test.want) {
			t.Errorf("mode %d: got %v, want %v", test.mode, got, test.want)
		}
	}
}

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
		_, err := Parse(NewFileBase(test.filename), strings.NewReader(test.src), nil, nil, test.mode)
		if ok := err == nil; ok != test.ok {
			t.Errorf("%s (mode %d): %q: got error %v, want ok = %v", test.filename, test.mode, test.src, err, test.ok)
		}
	}
}
