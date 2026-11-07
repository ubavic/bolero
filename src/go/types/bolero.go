// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"go/ast"
	"go/token"
	"internal/bolero"
	. "internal/types/errors"
)

// Declarations in Bolero source files may not be used from Go source files,
// in the same package or in any other package. Go source files remain plain
// Go, and Bolero code may use everything declared in Go source files.
//
// Package-level objects and methods declared in Bolero source files are
// recorded in their package. The marks are written while type-checking a
// package, or while importing it (through bolero.MarkGoTypesObject);
// afterwards they are only read, so concurrent use of imported packages
// is safe.

func init() {
	bolero.MarkGoTypesObject = func(obj any) { markBolero(obj.(Object)) }
}

// A boleroPackage holds the Bolero information about a package.
type boleroPackage struct {
	decls map[Object]bool         // package-level objects and methods declared in Bolero source files
	enums map[*TypeName]*enumInfo // enum types declared in the package
	props map[propKey][]property  // static properties

	// curObj and curDecl are the type declaration being checked, if it
	// declares static properties. They are nil otherwise.
	curObj  *TypeName
	curDecl *ast.TypeSpec
}

// boleroInfo returns the Bolero information about pkg, creating it if needed.
func (pkg *Package) boleroInfo() *boleroPackage {
	if pkg.bolero == nil {
		pkg.bolero = &boleroPackage{
			decls: make(map[Object]bool),
			enums: make(map[*TypeName]*enumInfo),
		}
	}
	return pkg.bolero
}

func isBolero(obj Object) bool {
	pkg := obj.Pkg()
	return pkg != nil && pkg.bolero != nil && pkg.bolero.decls[obj]
}

func markBolero(obj Object) {
	obj.Pkg().boleroInfo().decls[obj] = true
}

// markBoleroDecls marks the package-level objects and methods declared in
// Bolero source files.
func (check *Checker) markBoleroDecls() {
	for obj, d := range check.objMap {
		if check.isBoleroPos(d.file.pos) {
			markBolero(obj)
		}
	}
}

// checkBoleroUse reports whether the use of obj at e is permitted.
// Otherwise it reports an error: objects declared in Bolero source
// files cannot be used in Go source files.
func (check *Checker) checkBoleroUse(e *ast.Ident, obj Object) bool {
	if !isBolero(obj) || check.isBoleroPos(e.Pos()) {
		return true
	}
	check.errorf(e, UndeclaredName, "%s is declared in a Bolero file and cannot be used in Go files", obj.Name())
	return false
}

// isBoleroPos reports whether pos is in a Bolero source file.
// //line directives do not change the file a position is in.
func (check *Checker) isBoleroPos(pos token.Pos) bool {
	if check.fset == nil {
		return false
	}
	f := check.fset.File(pos)
	return f != nil && bolero.IsSourceFile(f.Name())
}
