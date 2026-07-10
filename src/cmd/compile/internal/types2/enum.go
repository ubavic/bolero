// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements type checking for enum types.
//
// Enums are declared in Bolero source files with the "enum" keyword in a
// defined type declaration:
//
//	type Color enum {
//		Red
//		Green
//		Blue
//	}
//
// The underlying type of an enum type is int. Its values are constants of
// the enum type, numbered from 0 in declaration order, and are accessed
// through the type name: Color::Red. Enum types support comparison,
// iteration over their values (for c := range Color), and methods, like
// other defined types.
//
// A value of an enum type is always one of its values:
//   - untyped constants are not implicitly converted to enum types;
//   - a constant converted to an enum type must be one of its values;
//   - only integers can be converted to enum types, and the compiler
//     checks conversions of non-constant values at run time;
//   - a switch on an enum value has a case for every value of the enum,
//     or a default clause.
//
// Enum types are recorded in the package that declares them, and export
// data carries them to importers (see SetEnumValues).

package types2

import (
	"cmd/compile/internal/syntax"
	"go/constant"
	. "internal/types/errors"
	"strings"
)

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

// EnumValues returns the names of the values of the enum type declared
// by tn, in declaration order, or nil if tn doesn't declare an enum type.
func EnumValues(tn *TypeName) []string {
	if pkg := tn.Pkg(); pkg != nil && pkg.bolero != nil {
		if info := pkg.bolero.enums[tn]; info != nil {
			return info.names
		}
	}
	return nil
}

// SetEnumValues records that tn declares an enum type with the given
// values. Importers call it while reading a package.
func SetEnumValues(tn *TypeName, names []string) {
	tn.Pkg().boleroInfo().enums[tn] = newEnumInfo(names)
}

// enumType type-checks an enum type literal, records the enum type
// declared by def, and returns the underlying type (int).
func (check *Checker) enumType(e *syntax.EnumType, def *TypeName) Type {
	if def == nil {
		check.error(e, InvalidSyntaxTree, "enum type may only appear in a defined type declaration")
		return Typ[Invalid]
	}
	if named, _ := def.typ.(*Named); named != nil && named.TypeParams().Len() > 0 {
		check.error(e, UnsupportedFeature, "enum types cannot have type parameters")
		return Typ[Invalid]
	}
	if len(e.ValueList) == 0 {
		check.error(e, InvalidSyntaxTree, "enum type must declare at least one value")
		return Typ[Invalid]
	}

	var names []string
	declared := make(map[string]bool)
	for _, name := range e.ValueList {
		switch {
		case name.Value == "_":
			check.error(name, BlankPkgName, "cannot use blank identifier as enum value")
		case declared[name.Value]:
			check.errorf(name, DuplicateDecl, "duplicate enum value %s", name.Value)
		default:
			declared[name.Value] = true
			names = append(names, name.Value)
		}
	}
	def.pkg.boleroInfo().enums[def] = newEnumInfo(names)

	return Typ[Int]
}

// enumAccess type-checks an expression of the form X::Sel, where X
// denotes an enum type and Sel is one of its values.
func (check *Checker) enumAccess(x *operand, e *syntax.EnumAccessExpr) {
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
		check.errorf(e.X, NotAnExpr, "%s is not an enum type", tx.typ)
		return
	}
	val := info.values[e.Sel.Value]
	if val == nil {
		check.errorf(e.Sel, UndeclaredName, "%s is not a value of enum %s", e.Sel.Value, tx.typ)
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
func (check *Checker) enumRange(x *operand, e syntax.Expr) bool {
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
func (check *Checker) isTypeName(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.Name:
		_, ok := check.lookup(e.Value).(*TypeName)
		return ok
	case *syntax.SelectorExpr:
		pkg, _ := e.X.(*syntax.Name)
		if pkg == nil {
			return false
		}
		pname, _ := check.lookup(pkg.Value).(*PkgName)
		if pname == nil {
			return false
		}
		_, ok := pname.imported.Scope().Lookup(e.Sel.Value).(*TypeName)
		return ok
	}
	return false
}

// enumConversion reports whether x may be converted to T, if T is an enum
// type; otherwise it reports an error and sets x.mode to invalid. Only
// integers can be converted to enum types, and constants must be values of
// the enum. The compiler checks non-constant conversions at run time.
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
// comparison operators are defined on enum types, so that operations
// cannot produce values that are not enum values.
func (check *Checker) enumOperand(x *operand, op syntax.Operator) bool {
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
func (check *Checker) enumSwitch(x *operand, s *syntax.SwitchStmt, seen valueMap) {
	if x.mode == invalid || s.Tag == nil {
		return
	}
	info := enumInfoOf(x.typ)
	if info == nil {
		return
	}
	for _, clause := range s.Body {
		if clause != nil && clause.Cases == nil {
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
