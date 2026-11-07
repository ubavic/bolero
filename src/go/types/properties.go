// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements static properties for go/types.
// It mirrors cmd/compile/internal/types2/properties.go. go/types checks
// the properties and field descriptors; the compiler generates the
// registration code and field-descriptor calls.

package types

import (
	"go/ast"
	"go/constant"
	"go/token"
	"internal/bolero"
	. "internal/types/errors"
	"math/big"
)

func init() {
	bolero.AddGoTypesProp = func(tn any, on byte, name, key string, val, typ any) {
		addBoleroProp(tn.(*TypeName), on, name, key, anyToConst(val), typ.(Type))
	}
}

type propOn uint8

const (
	propType propOn = iota
	propField
	propValue
)

type property struct {
	name string
	val  constant.Value
	typ  Type
}

type propKey struct {
	obj  *TypeName
	on   propOn
	name string
}

func (b *boleroPackage) setProps(obj *TypeName, on propOn, name string, props []property) {
	if b.props == nil {
		b.props = make(map[propKey][]property)
	}
	b.props[propKey{obj, on, name}] = props
}

func (b *boleroPackage) getProps(obj *TypeName, on propOn, name string) []property {
	if b == nil {
		return nil
	}
	return b.props[propKey{obj, on, name}]
}

func addBoleroProp(tn *TypeName, on byte, name, key string, val constant.Value, typ Type) {
	var kind propOn
	switch on {
	case 't':
		kind = propType
	case 'f':
		kind = propField
	case 'v':
		kind = propValue
	default:
		return
	}
	b := tn.Pkg().boleroInfo()
	b.setProps(tn, kind, name, append(b.getProps(tn, kind, name), property{key, val, typ}))
}

func anyToConst(v any) constant.Value {
	switch v := v.(type) {
	case bool:
		return constant.MakeBool(v)
	case string:
		return constant.MakeString(v)
	case int64:
		return constant.MakeInt64(v)
	case *big.Int:
		return constant.Make(v)
	case *big.Rat:
		return constant.Make(v)
	case *big.Float:
		return constant.Make(v)
	case bolero.ComplexVal:
		re := anyToConst(v.Re)
		im := anyToConst(v.Im)
		return constant.BinaryOp(re, token.ADD, constant.MakeImag(im))
	default:
		return constant.MakeUnknown()
	}
}

func astHasProps(decl *ast.TypeSpec) bool {
	if decl == nil {
		return false
	}
	if decl.Props != nil {
		return true
	}
	return astExprHasProps(decl.Type)
}

func astExprHasProps(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.StructType:
		if e.Fields == nil {
			return false
		}
		for _, f := range e.Fields.List {
			if f.Props != nil || astExprHasProps(f.Type) {
				return true
			}
		}
	case *ast.EnumType:
		if e.Values == nil {
			return false
		}
		for _, f := range e.Values.List {
			if f.Props != nil {
				return true
			}
		}
	case *ast.StarExpr:
		return astExprHasProps(e.X)
	case *ast.ParenExpr:
		return astExprHasProps(e.X)
	}
	return false
}

func (check *Checker) typeProperties(obj *TypeName, decl *ast.TypeSpec) {
	if decl.Props == nil || decl.Assign.IsValid() {
		return
	}
	if !check.propsAllowed(obj, decl.Props) {
		return
	}
	if !isStructOrEnum(obj) {
		check.error(decl.Props, BadDecl, "properties are only allowed on struct and enum types")
		return
	}
	props, ok := check.checkPropList(decl.Props, propType)
	if ok && len(props) > 0 {
		check.pkg.boleroInfo().setProps(obj, propType, "", props)
	}
}

func (check *Checker) fieldProperties(styp *ast.StructType, f *ast.Field) {
	b := check.pkg.bolero
	if f.Props == nil {
		return
	}
	if b == nil || b.curDecl == nil || b.curDecl.Type != styp || b.curObj == nil {
		check.error(f.Props, BadDecl, "field properties are only allowed on the struct type of a type declaration")
		return
	}
	if !check.propsAllowed(b.curObj, f.Props) {
		return
	}
	if len(f.Names) != 1 {
		check.error(f.Props, BadDecl, "property block must follow a single field")
		return
	}
	props, ok := check.checkPropList(f.Props, propField)
	if ok && len(props) > 0 {
		b.setProps(b.curObj, propField, f.Names[0].Name, props)
	}
}

func (check *Checker) enumProperties(def *TypeName, e *ast.EnumType) {
	if def == nil || e.Values == nil {
		return
	}
	for _, f := range e.Values.List {
		if f.Props == nil || len(f.Names) != 1 || f.Names[0].Name == "_" {
			continue
		}
		if !check.propsAllowed(def, f.Props) {
			continue
		}
		props, ok := check.checkPropList(f.Props, propValue)
		if ok && len(props) > 0 {
			def.pkg.boleroInfo().setProps(def, propValue, f.Names[0].Name, props)
		}
	}
}

func (check *Checker) propsAllowed(obj *TypeName, pos positioner) bool {
	if obj == nil || obj.parent != check.pkg.scope {
		check.error(pos, BadDecl, "properties are only allowed on package-level types")
		return false
	}
	if named, _ := obj.typ.(*Named); named != nil && named.TypeParams() != nil && named.TypeParams().Len() > 0 {
		check.error(pos, UnsupportedFeature, "generic types cannot have properties")
		return false
	}
	return true
}

func isStructOrEnum(obj *TypeName) bool {
	named, _ := obj.typ.(*Named)
	if named == nil || !isValid(named) {
		return false
	}
	if _, ok := named.Underlying().(*Struct); ok {
		return true
	}
	return enumInfoOf(named) != nil
}

func (check *Checker) checkPropList(list *ast.PropertyList, on propOn) ([]property, bool) {
	var out []property
	ok := true
	seen := make(map[string]bool)
	for _, prop := range list.List {
		if prop == nil || prop.Name == nil {
			continue
		}
		name := prop.Name.Name
		if name == "_" {
			check.error(prop.Name, BlankPkgName, "cannot use blank identifier as property name")
			ok = false
			continue
		}
		if seen[name] {
			check.errorf(prop.Name, DuplicateDecl, "duplicate property %s", name)
			ok = false
			continue
		}
		seen[name] = true
		val, typ, good := check.propValue(prop)
		if !good {
			ok = false
			continue
		}
		if on == propValue && name == "Label" && !isStringType(typ) {
			check.error(prop, BadDecl, "Label property must be a string")
			ok = false
			continue
		}
		out = append(out, property{name, val, typ})
	}
	if !ok {
		return nil, false
	}
	return out, true
}

func (check *Checker) propValue(prop *ast.Property) (constant.Value, Type, bool) {
	if prop.Value == nil {
		return constant.MakeBool(true), Typ[UntypedBool], true
	}
	var x operand
	check.expr(nil, &x, prop.Value)
	if x.mode == invalid {
		return nil, Typ[Invalid], false
	}
	if x.mode != constant_ {
		check.errorf(&x, InvalidConstInit, "property value %s is not constant", &x)
		return nil, Typ[Invalid], false
	}
	if !check.propFits(&x) {
		return nil, Typ[Invalid], false
	}
	return x.val, x.typ, true
}

func (check *Checker) propFits(x *operand) bool {
	if !isUntyped(x.typ) {
		return true
	}
	def := Default(x.typ)
	basic, _ := Unalias(def).(*Basic)
	if basic == nil {
		return true
	}
	if representableConst(x.val, check, basic, nil) {
		return true
	}
	check.errorf(x, NumericOverflow, "property value %s does not fit in %s", x.val, basic)
	return false
}

func isStringType(typ Type) bool {
	switch t := Unalias(typ).(type) {
	case *Basic:
		return t.kind == String || t.kind == UntypedString
	case *Named:
		if b, ok := t.Underlying().(*Basic); ok {
			return b.kind == String
		}
	}
	return false
}

func (check *Checker) propertyExpr(x *operand, e *ast.PropertyExpr) {
	x.mode = invalid
	x.expr = e
	props, ok := check.propPath(e.X)
	if !ok {
		return
	}
	for _, p := range props {
		if p.name == e.Name.Name {
			x.mode = constant_
			x.typ = p.typ
			x.val = p.val
			check.recordTypeAndValue(e, x.mode, x.typ, x.val)
			return
		}
	}
	check.errorf(e.Name, UndeclaredName, "%s has no property %s", exprPath(e.X), e.Name.Name)
}

func (check *Checker) propPath(e ast.Expr) ([]property, bool) {
	e = ast.Unparen(e)
	if acc, ok := e.(*ast.EnumAccessExpr); ok {
		var tx operand
		check.exprOrType(&tx, acc.X, true)
		if tx.mode == invalid {
			return nil, false
		}
		if tx.mode != typexpr {
			check.errorf(acc.X, NotAType, "%s is not a type", acc.X)
			return nil, false
		}
		if info := enumInfoOf(tx.typ); info != nil {
			if info.values[acc.Sel.Name] == nil {
				check.errorf(acc.Sel, UndeclaredName, "%s is not a value of enum %s", acc.Sel.Name, tx.typ)
				return nil, false
			}
			named, _ := Unalias(tx.typ).(*Named)
			return named.Obj().Pkg().bolero.getProps(named.Obj(), propValue, acc.Sel.Name), true
		}
		return check.lookupFieldProps(tx.typ, acc.Sel.Name, acc.Sel)
	}
	var tx operand
	check.exprOrType(&tx, e, true)
	if tx.mode == invalid {
		return nil, false
	}
	if tx.mode != typexpr {
		check.error(e, BadDecl, "property read requires a type, field, or enum value")
		return nil, false
	}
	named, _ := Unalias(tx.typ).(*Named)
	if named == nil || named.Obj().Pkg() == nil || named.Obj().Pkg().bolero == nil {
		return nil, true
	}
	return named.Obj().Pkg().bolero.getProps(named.Obj(), propType, ""), true
}

func (check *Checker) lookupFieldProps(T Type, name string, pos positioner) ([]property, bool) {
	obj, index, _ := LookupFieldOrMethod(T, false, check.pkg, name)
	fld, _ := obj.(*Var)
	if fld == nil || !fld.IsField() {
		if index != nil && obj == nil {
			check.errorf(pos, AmbiguousSelector, "ambiguous selector %s::%s", T, name)
		} else {
			check.errorf(pos, MissingFieldOrMethod, "%s is not a field of %s", name, T)
		}
		return nil, false
	}
	decl, field := declaringField(T, index)
	if decl == nil || decl.Obj().Pkg() == nil || decl.Obj().Pkg().bolero == nil {
		return nil, true
	}
	return decl.Obj().Pkg().bolero.getProps(decl.Obj(), propField, field.Name()), true
}

func declaringField(T Type, index []int) (*Named, *Var) {
	typ := T
	for i, ix := range index {
		if ptr, ok := typ.Underlying().(*Pointer); ok {
			typ = ptr.Elem()
		}
		named, _ := Unalias(typ).(*Named)
		st, ok := typ.Underlying().(*Struct)
		if !ok || ix >= st.NumFields() {
			return nil, nil
		}
		f := st.Field(ix)
		if i == len(index)-1 {
			return named, f
		}
		typ = f.Type()
	}
	return nil, nil
}

func (check *Checker) fieldDesc(x *operand, e *ast.EnumAccessExpr, T Type) {
	named, _ := Unalias(T).(*Named)
	if named == nil {
		check.errorf(e.X, NotAnExpr, "%s is not an enum type", T)
		return
	}
	if _, ok := named.Underlying().(*Struct); !ok {
		check.errorf(e.X, NotAnExpr, "%s is not an enum type", T)
		return
	}
	obj, index, _ := LookupFieldOrMethod(named, false, check.pkg, e.Sel.Name)
	fld, _ := obj.(*Var)
	if fld == nil || !fld.IsField() {
		if index != nil && obj == nil {
			check.errorf(e.Sel, AmbiguousSelector, "ambiguous selector %s::%s", T, e.Sel.Name)
		} else {
			check.errorf(e.Sel, MissingFieldOrMethod, "%s is not a field of %s", e.Sel.Name, T)
		}
		return
	}
	pn := check.metaImport()
	if pn == nil {
		check.errorf(e.Sel, UndeclaredName, "file must import \"bolero/meta\" to use %s", exprPath(e))
		return
	}
	check.usedPkgNames[pn] = true
	fieldObj := pn.imported.scope.Lookup("Field")
	ftn, _ := fieldObj.(*TypeName)
	if ftn == nil {
		check.error(e, InvalidDeclCycle, "bolero/meta.Field is not a type")
		return
	}
	inst, err := Instantiate(check.ctxt, ftn.Type(), []Type{named, fld.Type()}, true)
	if err != nil {
		check.errorf(e, InvalidDeclCycle, "cannot instantiate meta.Field: %s", err)
		return
	}
	x.mode = value
	x.typ = inst
	check.recordTypeAndValue(e, x.mode, x.typ, nil)
}

func (check *Checker) metaImport() *PkgName {
	for s := check.scope; s != nil; s = s.parent {
		for _, name := range s.Names() {
			pn, ok := s.Lookup(name).(*PkgName)
			if ok && pn.imported != nil && pn.imported.path == "bolero/meta" && pn.name != "_" {
				return pn
			}
		}
	}
	return nil
}

func exprPath(e ast.Expr) string {
	e = ast.Unparen(e)
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprPath(e.X) + "." + e.Sel.Name
	case *ast.EnumAccessExpr:
		return exprPath(e.X) + "::" + e.Sel.Name
	default:
		return "expression"
	}
}
