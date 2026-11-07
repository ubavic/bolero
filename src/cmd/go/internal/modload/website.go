// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modload

import (
	"context"
	"sync"
)

// loadExtraMu serializes packages added after the initial load.
// The module loader's work queue is not re-entered from those packages.
var loadExtraMu sync.Mutex

// LoadExtraPackage loads path into the current module loader.
// Page packages are imported only from generated code, so the initial
// load does not see them. Lookup succeeds after this returns.
func LoadExtraPackage(loaderstate *State, ctx context.Context, path string) error {
	loadExtraMu.Lock()
	defer loadExtraMu.Unlock()
	if loaded == nil {
		return errMissing
	}
	if pkg, ok := loaded.pkgCache.Get(path); ok {
		return pkg.err
	}
	loaded.pkg(loaderstate, ctx, path, 0)
	<-loaded.work.Idle()
	pkg, ok := loaded.pkgCache.Get(path)
	if !ok {
		return errMissing
	}
	return pkg.err
}
