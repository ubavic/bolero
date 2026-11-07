// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// bolero mod init

package modcmd

import (
	"cmd/go/internal/base"
	"cmd/go/internal/modload"
	"context"
)

var cmdInit = &base.Command{
	UsageLine: "bolero mod init [module-path]",
	Short:     "initialize new module in current directory",
	Long: `
Init initializes and writes a new app.json file in the current directory, in
effect creating a new module rooted at the current directory. An app.json or
go.mod file must not already exist there.

app.json is the manifest of a Bolero project. It holds the module path, the
Go version, module requirements, and the project's name, version, license
and authors. go.sum keeps its usual name and format. A Bolero project has
no go.mod file.

Init writes the module path, the Go version of this toolchain, version 0.1.0,
the MIT license, and a placeholder author.

With -modfile set to a file whose name is not app.json, init writes that file
in go.mod syntax instead. That is how a workspace module, which still uses
go.mod, is created.

Init accepts one optional argument, the module path for the new module. If the
module path argument is omitted, init will attempt to infer the module path
using import comments in .go files and the current directory (if in BOLERO_PATH).

See https://golang.org/ref/mod#go-mod-init for more about 'bolero mod init'.
`,
	Run: runInit,
}

func init() {
	base.AddChdirFlag(&cmdInit.Flag)
	base.AddModCommonFlags(&cmdInit.Flag)
}

func runInit(ctx context.Context, cmd *base.Command, args []string) {
	moduleLoaderState := modload.NewState()
	if len(args) > 1 {
		base.Fatalf("bolero: 'bolero mod init' accepts at most one argument")
	}
	var modPath string
	if len(args) == 1 {
		modPath = args[0]
	}

	moduleLoaderState.ForceUseModules = true
	modload.CreateModFile(moduleLoaderState, ctx, modPath) // does all the hard work
}
