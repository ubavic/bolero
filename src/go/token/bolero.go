// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package token

var boleroKeywords map[string]Token

func init() {
	boleroKeywords = make(map[string]Token)
	for i := bolero_beg + 1; i < bolero_end; i++ {
		if s := tokens[i]; s != "" && 'a' <= s[0] && s[0] <= 'z' {
			boleroKeywords[s] = i
		}
	}
}

// LookupBolero is like [Lookup], but also recognizes Bolero keywords.
// Bolero keywords are keywords only in Bolero source files; in Go source
// files they are ordinary identifiers.
func LookupBolero(ident string) Token {
	if tok, ok := boleroKeywords[ident]; ok {
		return tok
	}
	return Lookup(ident)
}
