// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements static properties.
//
// A property block attached to a package-level struct type, struct field,
// enum type or enum value holds constant metadata. Bolero code reads an
// entry at compile time with the @ operator. The compiler also generates
// an init that registers the entries with bolero/meta, and a field
// descriptor T::F has type meta.Field[T, FT].

package types2

import (
	"cmd/compile/internal/syntax"
	"go/constant"
	. "internal/types/errors"
	"strconv"
)

// propOn identifies what a property block is attached to.
type propOn uint8

const (
	propType propOn = iota
	propField
	propValue
)

// property is one static property. expr is the value expression in the
// source; it is nil when the entry is a bare flag, whose value is true.
type property struct {
	name string
	val  constant.Value
	typ  Type
	expr syntax.Expr
}

type propKey struct {
	obj  *TypeName
	on   propOn
	name string // field or enum value name; empty for a type
}

// Prop is one static property of a declaration.
type Prop struct {
	Name string
	Val  constant.Value
	Typ  Type
	Expr syntax.Expr
}

// NamedProps is the properties of one field or enum value.
type NamedProps struct {
	Name string
	List []Prop
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

// PropertiesOf returns the static properties recorded for tn.
func PropertiesOf(tn *TypeName) (typ []Prop, fields, values []NamedProps) {
	b := tn.Pkg().bolero
	if b == nil {
		return nil, nil, nil
	}
	var fieldNames, valueNames []string
	fieldsByName := make(map[string][]Prop)
	valuesByName := make(map[string][]Prop)
	for key, list := range b.props {
		if key.obj != tn {
			continue
		}
		out := exportProps(list)
		switch key.on {
		case propType:
			typ = out
		case propField:
			fieldNames = append(fieldNames, key.name)
			fieldsByName[key.name] = out
		case propValue:
			valueNames = append(valueNames, key.name)
			valuesByName[key.name] = out
		}
	}
	// Declaration order for fields and values follows the type, not the map.
	if named, _ := tn.Type().(*Named); named != nil {
		if st, ok := named.Underlying().(*Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				name := st.Field(i).Name()
				if list, ok := fieldsByName[name]; ok {
					fields = append(fields, NamedProps{name, list})
					delete(fieldsByName, name)
				}
			}
		}
		if info := enumInfoOf(tn.Type()); info != nil {
			for _, name := range info.names {
				if list, ok := valuesByName[name]; ok {
					values = append(values, NamedProps{name, list})
					delete(valuesByName, name)
				}
			}
		}
	}
	for _, name := range fieldNames {
		if list, ok := fieldsByName[name]; ok {
			fields = append(fields, NamedProps{name, list})
		}
	}
	for _, name := range valueNames {
		if list, ok := valuesByName[name]; ok {
			values = append(values, NamedProps{name, list})
		}
	}
	return typ, fields, values
}

func exportProps(list []property) []Prop {
	out := make([]Prop, len(list))
	for i, p := range list {
		out[i] = Prop{p.name, p.val, p.typ, p.expr}
	}
	return out
}

// StringLabel returns the Label property of enum value name of tn when
// that property is a string constant.
func StringLabel(tn *TypeName, name string) (string, bool) {
	b := tn.Pkg().bolero
	if b == nil {
		return "", false
	}
	for _, p := range b.getProps(tn, propValue, name) {
		if p.name == "Label" && isStringType(p.typ) {
			return constant.StringVal(p.val), true
		}
	}
	return "", false
}

// SetProperties records static properties read from export data.
// Importers call it while reading a package.
func SetProperties(tn *TypeName, typ []Prop, fields, values []NamedProps) {
	b := tn.Pkg().boleroInfo()
	if len(typ) > 0 {
		b.setProps(tn, propType, "", importProps(typ))
	}
	for _, f := range fields {
		b.setProps(tn, propField, f.Name, importProps(f.List))
	}
	for _, v := range values {
		b.setProps(tn, propValue, v.Name, importProps(v.List))
	}
}

func importProps(list []Prop) []property {
	out := make([]property, len(list))
	for i, p := range list {
		out[i] = property{name: p.Name, val: p.Val, typ: p.Typ}
	}
	return out
}

func syntaxHasProps(decl *syntax.TypeDecl) bool {
	if decl == nil || decl.Props != nil {
		return decl != nil && decl.Props != nil
	}
	return exprHasProps(decl.Type)
}

func exprHasProps(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.StructType:
		for _, f := range e.FieldList {
			if f.Props != nil || exprHasProps(f.Type) {
				return true
			}
		}
	case *syntax.EnumType:
		for _, p := range e.Props {
			if p != nil {
				return true
			}
		}
	case *syntax.Operation:
		return exprHasProps(e.X)
	case *syntax.ParenExpr:
		return exprHasProps(e.X)
	}
	return false
}

func (check *Checker) typeProperties(obj *TypeName, decl *syntax.TypeDecl) {
	if decl.Props == nil || decl.Alias {
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

func (check *Checker) fieldProperties(styp *syntax.StructType, f *syntax.Field) {
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
	if f.Name == nil {
		check.error(f.Props, BadDecl, "embedded fields cannot have properties")
		return
	}
	props, ok := check.checkPropList(f.Props, propField)
	if ok && len(props) > 0 {
		b.setProps(b.curObj, propField, f.Name.Value, props)
	}
}

func (check *Checker) enumProperties(def *TypeName, e *syntax.EnumType) {
	if def == nil {
		return
	}
	for i, name := range e.ValueList {
		if i >= len(e.Props) || e.Props[i] == nil {
			continue
		}
		if name.Value == "_" {
			continue
		}
		if !check.propsAllowed(def, e.Props[i]) {
			continue
		}
		props, ok := check.checkPropList(e.Props[i], propValue)
		if ok && len(props) > 0 {
			def.pkg.boleroInfo().setProps(def, propValue, name.Value, props)
		}
	}
}

func (check *Checker) propsAllowed(obj *TypeName, pos poser) bool {
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

func (check *Checker) checkPropList(list *syntax.PropertyList, on propOn) ([]property, bool) {
	var out []property
	ok := true
	seen := make(map[string]bool)
	for _, prop := range list.List {
		if prop == nil || prop.Name == nil {
			continue
		}
		name := prop.Name.Value
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
		out = append(out, property{name, val, typ, prop.Value})
	}
	if !ok {
		return nil, false
	}
	return out, true
}

func (check *Checker) propValue(prop *syntax.Property) (constant.Value, Type, bool) {
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

func (check *Checker) propertyExpr(x *operand, e *syntax.PropertyExpr) {
	x.mode = invalid
	x.expr = e
	props, ok := check.propPath(e.X)
	if !ok {
		return
	}
	for _, p := range props {
		if p.name == e.Name.Value {
			x.mode = constant_
			x.typ = p.typ
			x.val = p.val
			check.recordTypeAndValue(e, x.mode, x.typ, x.val)
			return
		}
	}
	check.errorf(e.Name, UndeclaredName, "%s has no property %s", syntax.String(e.X), e.Name.Value)
}

// propPath returns the properties of the static path e.
func (check *Checker) propPath(e syntax.Expr) ([]property, bool) {
	e = syntax.Unparen(e)
	if acc, ok := e.(*syntax.EnumAccessExpr); ok {
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
			if info.values[acc.Sel.Value] == nil {
				check.errorf(acc.Sel, UndeclaredName, "%s is not a value of enum %s", acc.Sel.Value, tx.typ)
				return nil, false
			}
			named, _ := Unalias(tx.typ).(*Named)
			return named.Obj().Pkg().bolero.getProps(named.Obj(), propValue, acc.Sel.Value), true
		}
		props, ok := check.lookupFieldProps(tx.typ, acc.Sel.Value, acc.Sel)
		return props, ok
	}
	if !check.isTypeName(e) && !isTypeExpr(e) {
		check.error(e, BadDecl, "property read requires a type, field, or enum value")
		return nil, false
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

func isTypeExpr(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.Name, *syntax.SelectorExpr:
		return true
	default:
		_ = e
		return false
	}
}

func (check *Checker) lookupFieldProps(T Type, name string, pos poser) ([]property, bool) {
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

// DescriptorProps describes the field selected by recv::name in pkg.
// props are the static properties of the field that declares it,
// which for a promoted field is the embedded field's own properties.
func DescriptorProps(recv Type, name string, pkg *Package) (field *Var, index []int, indirect bool, props []Prop) {
	obj, index, indirect := LookupFieldOrMethod(recv, false, pkg, name)
	field, _ = obj.(*Var)
	if field == nil || !field.IsField() {
		return nil, nil, false, nil
	}
	decl, f := declaringField(recv, index)
	if decl == nil || decl.Obj() == nil || decl.Obj().Pkg() == nil {
		return field, index, indirect, nil
	}
	_, fields, _ := PropertiesOf(decl.Obj())
	for _, fp := range fields {
		if fp.Name == f.Name() {
			props = fp.List
			break
		}
	}
	return field, index, indirect, props
}

// declaringField walks index from T and returns the named struct that
// declares the field, and the field.
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

// fieldDesc type-checks T::F as a field descriptor of type meta.Field[T, FT].
func (check *Checker) fieldDesc(x *operand, e *syntax.EnumAccessExpr, T Type) {
	named, _ := Unalias(T).(*Named)
	if named == nil {
		check.errorf(e.X, NotAnExpr, "%s is not an enum type", T)
		return
	}
	if _, ok := named.Underlying().(*Struct); !ok {
		check.errorf(e.X, NotAnExpr, "%s is not an enum type", T)
		return
	}
	obj, index, _ := LookupFieldOrMethod(named, false, check.pkg, e.Sel.Value)
	fld, _ := obj.(*Var)
	if fld == nil || !fld.IsField() {
		if index != nil && obj == nil {
			check.errorf(e.Sel, AmbiguousSelector, "ambiguous selector %s::%s", T, e.Sel.Value)
		} else {
			check.errorf(e.Sel, MissingFieldOrMethod, "%s is not a field of %s", e.Sel.Value, T)
		}
		return
	}
	pn := check.metaImport()
	if pn == nil {
		check.errorf(e.Sel, UndeclaredName, "file must import \"bolero/meta\" to use %s", syntax.String(e))
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

// metaImport returns the file's import of bolero/meta, if it is not blank.
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

// finishBolero fills the generated init functions that register static
// properties. It runs after package-level declarations are checked and
// before function bodies, so the registration code is type-checked.
func (check *Checker) finishBolero() {
	b := check.pkg.bolero
	if b == nil {
		return
	}
	for _, file := range check.files {
		if file.BoleroInit == nil {
			continue
		}
		var stmts []syntax.Stmt
		seen := make(map[*TypeName]bool)
		for key := range b.props {
			if key.obj == nil || seen[key.obj] || key.obj.Pos().FileBase() != file.Pos().FileBase() {
				continue
			}
			seen[key.obj] = true
			stmts = append(stmts, check.regProps(file, key.obj)...)
		}
		if len(stmts) == 0 {
			// The import exists so registration could be emitted. Mark it
			// used when every property in the file was rejected.
			for _, pn := range check.imports {
				if pn.imported != nil && pn.imported.path == "bolero/meta" && pn.Pos().FileBase() == file.Pos().FileBase() {
					check.usedPkgNames[pn] = true
				}
			}
			continue
		}
		file.BoleroInit.Body.List = stmts
	}
}

func (check *Checker) regProps(file *syntax.File, obj *TypeName) []syntax.Stmt {
	b := obj.pkg.bolero
	var stmts []syntax.Stmt
	if list := b.getProps(obj, propType, ""); len(list) > 0 {
		stmts = append(stmts, check.regCall(file, obj.Pos(), "RegisterType", []syntax.Expr{typeArg(obj)}, []syntax.Expr{propsCall(file, obj.Pos(), list)}))
	}
	_, _, values := PropertiesOf(obj)
	for _, v := range values {
		list := b.getProps(obj, propValue, v.Name)
		if len(list) == 0 {
			continue
		}
		val := enumAccessExpr(obj.Pos(), obj.Name(), v.Name)
		stmts = append(stmts, check.regCall(file, obj.Pos(), "RegisterValue", nil, []syntax.Expr{val, propsCall(file, obj.Pos(), list)}))
	}
	return stmts
}

func (check *Checker) regCall(file *syntax.File, pos syntax.Pos, name string, targs, args []syntax.Expr) syntax.Stmt {
	fun := metaName(file, pos, name)
	if len(targs) == 1 {
		fun = &syntax.IndexExpr{X: fun, Index: targs[0]}
		fun.SetPos(pos)
	} else if len(targs) > 1 {
		fun = &syntax.IndexExpr{X: fun, Index: &syntax.ListExpr{ElemList: targs}}
		fun.SetPos(pos)
	}
	call := &syntax.CallExpr{Fun: fun, ArgList: args}
	call.SetPos(pos)
	stmt := &syntax.ExprStmt{X: call}
	stmt.SetPos(pos)
	return stmt
}

func typeArg(obj *TypeName) syntax.Expr {
	return syntax.NewName(obj.Pos(), obj.Name())
}

func enumAccessExpr(pos syntax.Pos, typ, value string) syntax.Expr {
	e := &syntax.EnumAccessExpr{X: syntax.NewName(pos, typ), Sel: syntax.NewName(pos, value)}
	e.SetPos(pos)
	return e
}

func propsCall(file *syntax.File, pos syntax.Pos, list []property) syntax.Expr {
	var args []syntax.Expr
	for _, p := range list {
		args = append(args, stringLit(pos, p.name))
		if p.expr == nil {
			args = append(args, syntax.NewName(pos, "true"))
		} else {
			args = append(args, cloneExpr(p.expr))
		}
	}
	call := &syntax.CallExpr{Fun: metaName(file, pos, "NewProperties"), ArgList: args}
	call.SetPos(pos)
	return call
}

func metaName(file *syntax.File, pos syntax.Pos, name string) syntax.Expr {
	id := syntax.NewName(pos, name)
	if file.BoleroMeta == "" {
		return id
	}
	sel := &syntax.SelectorExpr{X: syntax.NewName(pos, file.BoleroMeta), Sel: id}
	sel.SetPos(pos)
	return sel
}

func stringLit(pos syntax.Pos, s string) syntax.Expr {
	lit := &syntax.BasicLit{Value: strconv.Quote(s), Kind: syntax.StringLit}
	lit.SetPos(pos)
	return lit
}

func cloneExpr(e syntax.Expr) syntax.Expr {
	if e == nil {
		return nil
	}
	switch e := e.(type) {
	case *syntax.Name:
		return syntax.NewName(e.Pos(), e.Value)
	case *syntax.BasicLit:
		n := &syntax.BasicLit{Value: e.Value, Kind: e.Kind, Bad: e.Bad}
		n.SetPos(e.Pos())
		return n
	case *syntax.ParenExpr:
		n := &syntax.ParenExpr{X: cloneExpr(e.X)}
		n.SetPos(e.Pos())
		return n
	case *syntax.Operation:
		n := &syntax.Operation{Op: e.Op, X: cloneExpr(e.X), Y: cloneExpr(e.Y)}
		n.SetPos(e.Pos())
		return n
	case *syntax.SelectorExpr:
		n := &syntax.SelectorExpr{X: cloneExpr(e.X), Sel: syntax.NewName(e.Sel.Pos(), e.Sel.Value)}
		n.SetPos(e.Pos())
		return n
	case *syntax.EnumAccessExpr:
		n := &syntax.EnumAccessExpr{X: cloneExpr(e.X), Sel: syntax.NewName(e.Sel.Pos(), e.Sel.Value)}
		n.SetPos(e.Pos())
		return n
	case *syntax.PropertyExpr:
		n := &syntax.PropertyExpr{X: cloneExpr(e.X), Name: syntax.NewName(e.Name.Pos(), e.Name.Value)}
		n.SetPos(e.Pos())
		return n
	case *syntax.CallExpr:
		args := make([]syntax.Expr, len(e.ArgList))
		for i, a := range e.ArgList {
			args[i] = cloneExpr(a)
		}
		n := &syntax.CallExpr{Fun: cloneExpr(e.Fun), ArgList: args, HasDots: e.HasDots}
		n.SetPos(e.Pos())
		return n
	case *syntax.IndexExpr:
		n := &syntax.IndexExpr{X: cloneExpr(e.X), Index: cloneExpr(e.Index)}
		n.SetPos(e.Pos())
		return n
	case *syntax.ListExpr:
		elems := make([]syntax.Expr, len(e.ElemList))
		for i, el := range e.ElemList {
			elems[i] = cloneExpr(el)
		}
		n := &syntax.ListExpr{ElemList: elems}
		n.SetPos(e.Pos())
		return n
	default:
		// A constant expression outside this set still has to be type-checked
		// in the registration init. Sharing the node would overwrite the
		// type information recorded on the property itself.
		return e
	}
}
