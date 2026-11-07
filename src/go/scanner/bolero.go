// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scanner

import "go/token"

// lookup maps an identifier to its keyword token in the scanner's mode,
// or to [token.IDENT] if it is not a keyword.
func (s *Scanner) lookup(ident string) token.Token {
	if s.mode&Bolero != 0 {
		return token.LookupBolero(ident)
	}
	return token.Lookup(ident)
}
