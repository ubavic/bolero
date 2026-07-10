// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"internal/pkgbits"

	"cmd/compile/internal/types2"
)

// Export data records which objects and methods are declared in Bolero
// source files in the optional Bolero root (see pkgbits.BoleroRootIdx),
// so that importers can keep Go source files from using them.

// writeBoleroRoot writes the Bolero root of the local package's
// bitstream, if any of its objects or methods are declared in Bolero
// source files.
func (pw *pkgWriter) writeBoleroRoot() {
	objs := make(map[index]pkgbits.BoleroObj)
	for obj, idx := range pw.objsIdx {
		if obj.Pkg() != pw.curpkg {
			continue
		}
		bo := pkgbits.BoleroObj{Decl: types2.IsBolero(obj)}
		if named := definedNamed(obj); named != nil {
			for i := range named.NumMethods() {
				if m := named.Method(i); types2.IsBolero(m) {
					bo.Methods = append(bo.Methods, m.Name())
				}
			}
			bo.Enum = types2.EnumValues(named.Obj())
		}
		if bo.Decl || len(bo.Methods) > 0 || len(bo.Enum) > 0 {
			objs[idx] = bo
		}
	}
	if len(objs) > 0 {
		pw.WriteBoleroRoot(objs)
	}
}

// definedNamed returns the defined type declared by obj, or nil.
func definedNamed(obj types2.Object) *types2.Named {
	tn, ok := obj.(*types2.TypeName)
	if !ok || tn.IsAlias() {
		return nil
	}
	named, _ := tn.Type().(*types2.Named)
	return named
}

// boleroLinker carries the Bolero roots of the linked bitstreams over
// to the export data.
type boleroLinker struct {
	roots map[*pkgReader]map[index]pkgbits.BoleroObj
	objs  map[index]pkgbits.BoleroObj
}

// relocBolero records the Bolero declarations of object idx of pr,
// which was copied to index newidx of the export data.
func (l *linker) relocBolero(pr *pkgReader, idx, newidx index) {
	b := &l.bolero
	if b.roots == nil {
		b.roots = make(map[*pkgReader]map[index]pkgbits.BoleroObj)
		b.objs = make(map[index]pkgbits.BoleroObj)
	}
	root, ok := b.roots[pr]
	if !ok {
		root = pr.BoleroRoot()
		b.roots[pr] = root
	}
	if obj, ok := root[idx]; ok {
		b.objs[newidx] = obj
	}
}

// writeBoleroRoot writes the Bolero root of the export data, if any
// copied object or method is declared in a Bolero source file.
func (l *linker) writeBoleroRoot() {
	if len(l.bolero.objs) > 0 {
		l.pw.WriteBoleroRoot(l.bolero.objs)
	}
}
