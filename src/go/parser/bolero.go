// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"go/scanner"
	"go/token"
	"internal/bolero"
)

// fileMode returns mode, with Bolero syntax enabled if file is a Bolero source file.
func fileMode(file *token.File, mode Mode) Mode {
	if bolero.IsSourceFile(file.Name()) {
		mode |= Bolero
	}
	return mode
}

// scannerMode returns the scanner mode used by a parser in the given mode.
func scannerMode(mode Mode) scanner.Mode {
	m := scanner.ScanComments
	if mode&Bolero != 0 {
		m |= scanner.Bolero
	}
	return m
}
