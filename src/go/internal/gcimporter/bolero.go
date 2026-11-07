// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcimporter

import (
	"go/constant"
	"go/types"
	"internal/bolero"
	"internal/pkgbits"
	"math/big"
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
		if len(bo.Enum) > 0 {
			bolero.SetGoTypesEnum(obj, bo.Enum)
		}
		if len(bo.Props) > 0 && bolero.AddGoTypesProp != nil {
			for _, p := range bo.Props {
				bolero.AddGoTypesProp(obj, p.On, p.Name, p.Key, constToAny(p.Val), pr.boleroConstType(p))
			}
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

func (pr *pkgReader) boleroConstType(p pkgbits.BoleroProp) types.Type {
	if p.Basic >= 0 {
		return types.Typ[types.BasicKind(p.Basic)]
	}
	pkg := pr.imports[p.TypePkg]
	if pkg == nil {
		return types.Typ[types.Invalid]
	}
	obj := pkg.Scope().Lookup(p.TypeName)
	if obj == nil {
		return types.Typ[types.Invalid]
	}
	return obj.Type()
}

func constToAny(v constant.Value) any {
	switch v.Kind() {
	case constant.Bool:
		return constant.BoolVal(v)
	case constant.String:
		return constant.StringVal(v)
	case constant.Int:
		if i, ok := constant.Int64Val(v); ok {
			return i
		}
		return constant.Val(v).(*big.Int)
	case constant.Float:
		return constant.Val(v).(*big.Float)
	case constant.Complex:
		return bolero.ComplexVal{Re: constToAny(constant.Real(v)), Im: constToAny(constant.Imag(v))}
	default:
		if r, ok := constant.Val(v).(*big.Rat); ok {
			return r
		}
		return constant.Val(v)
	}
}
