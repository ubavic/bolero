// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements type checking for enum types.
// It mirrors cmd/compile/internal/types2/enum.go, which documents the
// semantics, using go/ast in place of cmd/compile/internal/syntax.

package types

import (
	"go/ast"
	"go/constant"
	"go/token"
	"internal/bolero"
	. "internal/types/errors"
	"slices"
	"strings"
)

func init() {
	bolero.SetGoTypesEnum = func(tn any, names []string) { setEnumValues(tn.(*TypeName), names) }
}

// enumInfo describes the values of an enum type.
type enumInfo struct {
	names  []string                  // value names in declaration order
	values map[string]constant.Value // name -> constant integer value
}

func newEnumInfo(names []string) *enumInfo {
	info := &enumInfo{names: names, values: make(map[string]constant.Value, len(names))}
	for i, name := range names {
		info.values[name] = constant.MakeInt64(int64(i))
	}
	return info
}

// enumInfoOf returns the enum information of typ, or nil if typ is not an enum type.
func enumInfoOf(typ Type) *enumInfo {
	named, _ := Unalias(typ).(*Named)
	if named == nil {
		return nil
	}
	obj := named.Obj()
	if pkg := obj.Pkg(); pkg != nil && pkg.bolero != nil {
		return pkg.bolero.enums[obj]
	}
	return nil
}

// setEnumValues records that tn declares an enum type with the given
// values. Importers call it, through bolero.SetGoTypesEnum, while reading
// a package.
func setEnumValues(tn *TypeName, names []string) {
	tn.Pkg().boleroInfo().enums[tn] = newEnumInfo(names)
}

// enumType type-checks an enum type literal, records the enum type
// declared by def, and returns the underlying type (int).
func (check *Checker) enumType(e *ast.EnumType, def *TypeName) Type {
	if def == nil {
		check.error(e, InvalidSyntaxTree, "enum type may only appear in a defined type declaration")
		return Typ[Invalid]
	}
	if named, _ := def.typ.(*Named); named != nil && named.TypeParams().Len() > 0 {
		check.error(e, UnsupportedFeature, "enum types cannot have type parameters")
		return Typ[Invalid]
	}
	var idents []*ast.Ident
	if e.Values != nil {
		for _, field := range e.Values.List {
			idents = append(idents, field.Names...)
		}
	}
	if len(idents) == 0 {
		check.error(e, InvalidSyntaxTree, "enum type must declare at least one value")
		return Typ[Invalid]
	}

	var names []string
	declared := make(map[string]bool)
	for _, name := range idents {
		switch {
		case name.Name == "_":
			check.error(name, BlankPkgName, "cannot use blank identifier as enum value")
		case declared[name.Name]:
			check.errorf(name, DuplicateDecl, "duplicate enum value %s", name.Name)
		default:
			declared[name.Name] = true
			names = append(names, name.Name)
		}
	}
	def.pkg.boleroInfo().enums[def] = newEnumInfo(names)
	check.enumProperties(def, e)
	if def.parent == check.pkg.scope && !slices.ContainsFunc(check.methods[def], func(m *Func) bool { return m.name == "String" }) {
		check.addEnumString(def)
	}

	return Typ[Int]
}

// addEnumString adds the String method that the compiler generates for the
// package-level enum type def, unless the package declares one (see
// cmd/compile/internal/noder.addEnumStringMethods). go/types needs only the
// method object, not its body.
func (check *Checker) addEnumString(def *TypeName) {
	named := def.typ.(*Named)
	recv := newVar(RecvVar, def.pos, def.pkg, "", named)
	result := NewTuple(newVar(ResultVar, def.pos, def.pkg, "", Typ[String]))
	m := NewFunc(def.pos, def.pkg, "String", NewSignatureType(recv, nil, nil, nil, result, false))
	named.AddMethod(m)
	markBolero(m)
}

// enumAccess type-checks an expression of the form X::Sel, where X
// denotes an enum type and Sel is one of its values.
func (check *Checker) enumAccess(x *operand, e *ast.EnumAccessExpr) {
	x.mode = invalid
	x.expr = e

	var tx operand
	check.exprOrType(&tx, e.X, true)
	switch {
	case tx.mode == invalid:
		return
	case tx.mode != typexpr:
		check.errorf(e.X, NotAType, "%s is not a type", e.X)
		return
	}

	info := enumInfoOf(tx.typ)
	if info == nil {
		check.fieldDesc(x, e, tx.typ)
		return
	}
	val := info.values[e.Sel.Name]
	if val == nil {
		check.errorf(e.Sel, UndeclaredName, "%s is not a value of enum %s", e.Sel.Name, tx.typ)
		return
	}

	x.mode = constant_
	x.typ = tx.typ
	x.val = val
	check.recordTypeAndValue(e, x.mode, x.typ, x.val)
}

// enumRange reports whether the range expression e denotes an enum type.
// If so, it sets x to a constant of the enum type whose value is the number
// of enum values, so that ranging over it yields the values in order.
func (check *Checker) enumRange(x *operand, e ast.Expr) bool {
	if !check.isTypeName(e) {
		return false
	}
	var tx operand
	check.exprOrType(&tx, e, false)
	if tx.mode != typexpr {
		return false
	}
	info := enumInfoOf(tx.typ)
	if info == nil {
		return false
	}
	x.mode = constant_
	x.typ = tx.typ
	x.val = constant.MakeInt64(int64(len(info.names)))
	x.expr = e
	check.record(x)
	return true
}

// isTypeName reports whether e is an identifier or a qualified identifier
// denoting a type. It records nothing.
func (check *Checker) isTypeName(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		_, ok := check.lookup(e.Name).(*TypeName)
		return ok
	case *ast.SelectorExpr:
		pkg, _ := e.X.(*ast.Ident)
		if pkg == nil {
			return false
		}
		pname, _ := check.lookup(pkg.Name).(*PkgName)
		if pname == nil {
			return false
		}
		_, ok := pname.imported.Scope().Lookup(e.Sel.Name).(*TypeName)
		return ok
	}
	return false
}

// enumConversion reports whether x may be converted to T, if T is an enum
// type; otherwise it reports an error and sets x.mode to invalid.
func (check *Checker) enumConversion(x *operand, T Type) bool {
	info := enumInfoOf(T)
	if info == nil || x.mode == invalid {
		return true
	}
	if !allInteger(x.typ) {
		check.errorf(x, InvalidConversion, "cannot convert %s to enum type %s: only integers can be converted to enum types", x, T)
		x.mode = invalid
		return false
	}
	if x.mode == constant_ {
		v, ok := constant.Int64Val(constant.ToInt(x.val))
		if !ok || v < 0 || v >= int64(len(info.names)) {
			check.errorf(x, InvalidConversion, "cannot convert %s to enum type %s: %s is not a value of %s", x, T, x.val, T)
			x.mode = invalid
			return false
		}
	}
	return true
}

// enumOperand reports whether x may be an operand of the arithmetic,
// bitwise or shift operator op; otherwise it reports an error. Only
// comparison operators are defined on enum types.
func (check *Checker) enumOperand(x *operand, op token.Token) bool {
	if enumInfoOf(x.typ) == nil {
		return true
	}
	check.errorf(x, UndefinedOp, invalidOp+"operator %s not defined on %s: enum types only support comparison", op, x)
	return false
}

// enumUnderlying returns the underlying type of typ if typ is an enum type,
// and typ otherwise. An untyped constant only gets an enum type through an
// explicit conversion, which enumConversion already checked; afterwards it
// only needs to be representable as the underlying type.
func enumUnderlying(typ Type) Type {
	if enumInfoOf(typ) != nil {
		return typ.Underlying()
	}
	return typ
}

// enumSwitch reports an error if the expression switch s on x, whose
// values seen were collected by caseValues, switches on an enum value but
// has neither a case for every value of the enum nor a default clause.
func (check *Checker) enumSwitch(x *operand, s *ast.SwitchStmt, seen valueMap) {
	if x.mode == invalid || s.Tag == nil {
		return
	}
	info := enumInfoOf(x.typ)
	if info == nil {
		return
	}
	for _, c := range s.Body.List {
		if clause, _ := c.(*ast.CaseClause); clause != nil && clause.List == nil {
			return // default clause
		}
	}
	var missing []string
	for i, name := range info.names {
		if !seenValue(seen, int64(i), x.typ) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		check.errorf(x, InvalidExprSwitch, "switch on %s has no case for %s and no default case", x.typ, strings.Join(missing, ", "))
	}
}

// seenValue reports whether the integer constant v of type typ is among
// the case values seen.
func seenValue(seen valueMap, v int64, typ Type) bool {
	for _, vt := range seen[v] {
		if Identical(vt.typ, typ) {
			return true
		}
	}
	return false
}
