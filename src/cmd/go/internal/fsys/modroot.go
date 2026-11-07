// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fsys

import (
	"internal/bolero"
	"path/filepath"
)

// HasModuleFile reports whether dir contains a module manifest,
// either a go.mod file or a Bolero app.json file.
func HasModuleFile(dir string) bool {
	return isRegular(filepath.Join(dir, "go.mod")) || isRegular(filepath.Join(dir, bolero.AppJSON))
}

func isRegular(path string) bool {
	fi, err := Stat(path)
	return err == nil && !fi.IsDir()
}
