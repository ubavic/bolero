// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package importer

import (
	"slices"

	"cmd/compile/internal/types2"
	"internal/pkgbits"
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
			types2.MarkBolero(obj)
		}
		if len(bo.Enum) > 0 {
			types2.SetEnumValues(obj.(*types2.TypeName), bo.Enum)
		}
		if len(bo.Props) > 0 {
			pr.setBoleroProps(obj.(*types2.TypeName), bo.Props)
		}
		if len(bo.Methods) == 0 {
			continue
		}
		named := obj.Type().(*types2.Named)
		for i := range named.NumMethods() {
			if m := named.Method(i); slices.Contains(bo.Methods, m.Name()) {
				types2.MarkBolero(m)
			}
		}
	}
}

func (pr *pkgReader) setBoleroProps(tn *types2.TypeName, props []pkgbits.BoleroProp) {
	var typ []types2.Prop
	var fields, values []types2.NamedProps
	put := func(on byte, name string, p types2.Prop) {
		switch on {
		case 't':
			typ = append(typ, p)
		case 'f':
			fields = appendProp(fields, name, p)
		case 'v':
			values = appendProp(values, name, p)
		}
	}
	for _, raw := range props {
		put(raw.On, raw.Name, types2.Prop{Name: raw.Key, Val: raw.Val, Typ: pr.boleroConstType(raw)})
	}
	types2.SetProperties(tn, typ, fields, values)
}

func appendProp(list []types2.NamedProps, name string, p types2.Prop) []types2.NamedProps {
	for i := range list {
		if list[i].Name == name {
			list[i].List = append(list[i].List, p)
			return list
		}
	}
	return append(list, types2.NamedProps{Name: name, List: []types2.Prop{p}})
}

func (pr *pkgReader) boleroConstType(p pkgbits.BoleroProp) types2.Type {
	if p.Basic >= 0 {
		return types2.Typ[types2.BasicKind(p.Basic)]
	}
	pkg := pr.imports[p.TypePkg]
	if pkg == nil {
		return types2.Typ[types2.Invalid]
	}
	obj := pkg.Scope().Lookup(p.TypeName)
	if obj == nil {
		return types2.Typ[types2.Invalid]
	}
	return obj.Type()
}
