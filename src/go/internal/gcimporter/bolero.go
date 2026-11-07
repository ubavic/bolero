// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcimporter

import (
	"go/types"
	"internal/bolero"
	"slices"
)

// markBolero marks the objects and methods that the Bolero root of the
// export data lists as declared in Bolero source files. Marking happens
// eagerly, while the package is read, so that the marks are only read
// once the package is complete.
func (pr *pkgReader) markBolero() {
	for idx, bo := range pr.BoleroRoot() {
		objPkg, objName := pr.objIdx(idx)
		obj := objPkg.Scope().Lookup(objName)
		if bo.Decl {
			bolero.MarkGoTypesObject(obj)
		}
		if len(bo.Methods) == 0 {
			continue
		}
		named := obj.Type().(*types.Named)
		for i := range named.NumMethods() {
			if m := named.Method(i); slices.Contains(bo.Methods, m.Name()) {
				bolero.MarkGoTypesObject(m)
			}
		}
	}
}
