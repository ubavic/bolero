// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import "internal/bolero"

// fileMode returns mode, with Bolero syntax enabled if file is a Bolero source file.
func fileMode(file *PosBase, mode Mode) Mode {
	if bolero.IsSourceFile(file.Filename()) {
		mode |= Bolero
	}
	return mode
}

// isKeywordToken reports whether tok is a Go or a Bolero keyword.
// Bolero tokens are declared after _Var; those spelled like
// identifiers are Bolero keywords.
func isKeywordToken(tok token) bool {
	if tok < _Break {
		return false
	}
	s := tok.String()
	return s != "" && isLetter(rune(s[0]))
}

// isKeyword reports whether the keyword token tok is a keyword
// in the scanner's mode.
func (s *scanner) isKeyword(tok token) bool {
	return tok <= _Var || s.mode&boleroTok != 0
}

// scannerMode returns the scanner mode used by a parser in the given mode.
func scannerMode(mode Mode) uint {
	m := directives
	if mode&Bolero != 0 {
		m |= boleroTok
	}
	return m
}
