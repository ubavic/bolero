// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fmtcmd implements the “bolero fmt” command.
package fmtcmd

import (
	"context"
	"errors"
	"fmt"
	"internal/bolero"
	"os"
	"path/filepath"

	"cmd/go/internal/base"
	"cmd/go/internal/cfg"
	"cmd/go/internal/load"
	"cmd/go/internal/modload"
	"cmd/internal/sys"
)

func init() {
	base.AddBuildFlagsNX(&CmdFmt.Flag)
	base.AddChdirFlag(&CmdFmt.Flag)
	base.AddModFlag(&CmdFmt.Flag)
	base.AddModCommonFlags(&CmdFmt.Flag)
}

var CmdFmt = &base.Command{
	Run:       runFmt,
	UsageLine: "bolero fmt [-n] [-x] [packages]",
	Short:     "bolerofmt (reformat) package sources",
	Long: `
Fmt runs the command 'bolerofmt -l -w' on the packages named
by the import paths. It prints the names of the files that are modified.

For more about bolerofmt, see 'bolero doc cmd/gofmt'.
For more about specifying packages, see 'bolero help packages'.

The -n flag prints commands that would be executed.
The -x flag prints commands as they are executed.

The -mod flag's value sets which module download mode
to use: readonly or vendor. See 'bolero help modules' for more.

To run bolerofmt with specific options, run bolerofmt itself.

See also: bolero fix, bolero vet.
	`,
}

func runFmt(ctx context.Context, cmd *base.Command, args []string) {
	moduleLoaderState := modload.NewState()
	printed := false
	gofmt := gofmtPath()

	gofmtArgs := []string{gofmt, "-l", "-w"}
	gofmtArgLen := len(gofmt) + len(" -l -w")

	baseGofmtArgs := len(gofmtArgs)
	baseGofmtArgLen := gofmtArgLen

	for _, pkg := range load.PackagesAndErrors(moduleLoaderState, ctx, load.PackageOpts{}, args) {
		if moduleLoaderState.Enabled() && pkg.Module != nil && !pkg.Module.Main {
			if !printed {
				fmt.Fprintf(os.Stderr, "bolero: not formatting packages in dependency modules\n")
				printed = true
			}
			continue
		}
		if pkg.Error != nil {
			if _, ok := errors.AsType[*load.NoGoError](pkg.Error); ok {
				// Skip this error, as we will format all files regardless.
			} else if _, ok := errors.AsType[*load.EmbedError](pkg.Error); ok && len(pkg.InternalAllGoFiles()) > 0 {
				// Skip this error, as we will format all files regardless.
			} else {
				base.Errorf("%v", pkg.Error)
				continue
			}
		}
		// Use pkg.gofiles instead of pkg.Dir so that
		// the command only applies to this package,
		// not to packages in subdirectories.
		files := base.RelPaths(pkg.InternalAllGoFiles())
		for _, file := range files {
			gofmtArgs = append(gofmtArgs, file)
			gofmtArgLen += 1 + len(file) // plus separator
			if gofmtArgLen >= sys.ExecArgLengthLimit {
				base.Run(gofmtArgs)
				gofmtArgs = gofmtArgs[:baseGofmtArgs]
				gofmtArgLen = baseGofmtArgLen
			}
		}
	}
	if len(gofmtArgs) > baseGofmtArgs {
		base.Run(gofmtArgs)
	}
}

func gofmtPath() string {
	gofmt := bolero.FormatCommandName + cfg.ToolExeSuffix()

	gofmtPath := filepath.Join(cfg.GOBIN, gofmt)
	if _, err := os.Stat(gofmtPath); err == nil {
		return gofmtPath
	}

	gofmtPath = filepath.Join(cfg.GOROOT, "bin", gofmt)
	if _, err := os.Stat(gofmtPath); err == nil {
		return gofmtPath
	}

	// fallback to looking for the formatter in $PATH
	return bolero.FormatCommandName
}
