// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"strconv"

	"cmd/compile/internal/ir"
	"cmd/compile/internal/syntax"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
	"cmd/compile/internal/types2"
)

// addEnumStringMethods gives every enum type declared at package level a
// String method that returns the name of the value, unless the package
// declares a String method on the type itself:
//
//	func (e Color) String() string { return [...]string{"Red", "Green", "Blue"}[e] }
//
// The method is added to the syntax trees before type checking, so it is
// compiled, exported and inlined like a method written by hand. go/types
// adds a matching method object (see go/types.Checker.enumType).
func addEnumStringMethods(noders []*noder) {
	declared := make(map[string]bool)
	for _, p := range noders {
		for _, decl := range p.file.DeclList {
			if fn, ok := decl.(*syntax.FuncDecl); ok && fn.Recv != nil && fn.Name.Value == "String" {
				declared[recvBaseName(fn.Recv.Type)] = true
			}
		}
	}
	for _, p := range noders {
		for _, decl := range p.file.DeclList {
			typ, ok := decl.(*syntax.TypeDecl)
			if !ok || typ.Alias || typ.TParamList != nil || declared[typ.Name.Value] {
				continue
			}
			if enum, ok := typ.Type.(*syntax.EnumType); ok {
				p.file.DeclList = append(p.file.DeclList, enumStringMethod(typ.Name, enum))
			}
		}
	}
}

// recvBaseName returns the name of the base type of the receiver type t,
// or "" if t is not a (pointer to a) possibly instantiated type name.
func recvBaseName(t syntax.Expr) string {
	for {
		switch x := t.(type) {
		case *syntax.Name:
			return x.Value
		case *syntax.Operation:
			if x.Op != syntax.Mul || x.Y != nil {
				return ""
			}
			t = x.X
		case *syntax.ParenExpr:
			t = x.X
		case *syntax.IndexExpr:
			t = x.X
		default:
			return ""
		}
	}
}

// enumStringMethod returns the String method of the enum type declared as
// name, at the position of name.
func enumStringMethod(name *syntax.Name, enum *syntax.EnumType) *syntax.FuncDecl {
	pos := name.Pos()
	var values []syntax.Expr
	for _, v := range enum.ValueList {
		values = append(values, at(pos, &syntax.BasicLit{Value: strconv.Quote(v.Value), Kind: syntax.StringLit}))
	}
	table := at(pos, &syntax.CompositeLit{
		Type:     at(pos, &syntax.ArrayType{Elem: syntax.NewName(pos, "string")}),
		ElemList: values,
		Rbrace:   pos,
	})
	ret := at(pos, &syntax.ReturnStmt{Results: at(pos, &syntax.IndexExpr{X: table, Index: syntax.NewName(pos, "e")})})

	return at(pos, &syntax.FuncDecl{
		Recv: at(pos, &syntax.Field{Name: syntax.NewName(pos, "e"), Type: syntax.NewName(pos, name.Value)}),
		Name: syntax.NewName(pos, "String"),
		Type: at(pos, &syntax.FuncType{ResultList: []*syntax.Field{at(pos, &syntax.Field{Type: syntax.NewName(pos, "string")})}}),
		Body: at(pos, &syntax.BlockStmt{List: []syntax.Stmt{ret}, Rbrace: pos}),
	})
}

// at sets the position of n to pos and returns n.
func at[N interface{ SetPos(syntax.Pos) }](pos syntax.Pos, n N) N {
	n.SetPos(pos)
	return n
}

// Converting a non-constant integer x to an enum type T with n values is
// checked at run time. The conversion T(x) becomes
//
//	T(runtime.boleroEnumValue(uint64(x), n, signed, "pkg.T"))
//
// which panics unless x is one of the enum's values. Sign extension makes
// negative signed values fail the check.

// enumConvertExpr writes the conversion of expr to dst, if dst is an enum
// type and the conversion needs a run-time check, and reports whether it did.
func (w *writer) enumConvertExpr(dst types2.Type, expr syntax.Expr) bool {
	named, _ := types2.Unalias(dst).(*types2.Named)
	if named == nil {
		return false
	}
	values := types2.EnumValues(named.Obj())
	if values == nil || types2.Identical(w.p.typeOf(expr), dst) {
		return false
	}

	w.Code(exprEnumConvert)
	w.pos(expr)
	w.typ(dst)
	w.Uint64(uint64(len(values)))
	w.String(types2.TypeString(dst, func(pkg *types2.Package) string { return pkg.Name() }))
	w.expr(expr)
	return true
}

// enumConvert reads a conversion written by enumConvertExpr.
func (r *reader) enumConvert() ir.Node {
	pos := r.pos()
	typ := r.typ()
	n := r.Uint64()
	name := r.String()
	x := typecheck.DefaultLit(r.expr(), nil)

	args := []ir.Node{
		typecheck.Conv(x, types.Types[types.TUINT64]),
		ir.NewInt(pos, int64(n)),
		ir.NewBool(pos, x.Type().IsSigned()),
		ir.NewString(pos, name),
	}
	call := typecheck.Call(pos, typecheck.LookupRuntime("boleroEnumValue"), args, false)
	return typecheck.Conv(call, typ)
}
