// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bolero

// Names of the commands installed in the root of the toolchain.
const (
	// CommandName is the name of the command that builds, runs and tests
	// programs. Its source is cmd/go.
	CommandName = "bolero"

	// FormatCommandName is the name of the source formatter.
	// Its source is cmd/gofmt.
	FormatCommandName = "bolerofmt"
)

// ExecName returns the name of the executable installed for the standard
// command with the given import path, and reports whether the toolchain
// names it differently from the last element of the path.
func ExecName(importPath string) (name string, ok bool) {
	switch importPath {
	case "cmd/go":
		return CommandName, true
	case "cmd/gofmt":
		return FormatCommandName, true
	}
	return "", false
}
