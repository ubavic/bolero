// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"fmt"
	"go/constant"
	"strconv"

	"cmd/compile/internal/syntax"
	"cmd/compile/internal/types2"
)

// prepareProperties gives every Bolero file that declares static properties
// an import of bolero/meta and an empty init. finishBolero fills the init
// once the property values are known.
func prepareProperties(noders []*noder) {
	for _, p := range noders {
		if !fileHasProps(p.file) {
			continue
		}
		p.file.BoleroMeta = ensureMetaImport(p.file)
		pos := p.file.PkgName.Pos()
		fn := at(pos, &syntax.FuncDecl{
			Name: syntax.NewName(pos, "init"),
			Type: at(pos, &syntax.FuncType{}),
			Body: at(pos, &syntax.BlockStmt{Rbrace: pos}),
		})
		// Place the init after the imports and before other declarations so
		// it runs before init functions written in the same file.
		i := 0
		for i < len(p.file.DeclList) {
			if _, ok := p.file.DeclList[i].(*syntax.ImportDecl); !ok {
				break
			}
			i++
		}
		p.file.DeclList = append(p.file.DeclList, nil)
		copy(p.file.DeclList[i+1:], p.file.DeclList[i:])
		p.file.DeclList[i] = fn
		p.file.BoleroInit = fn
	}
}

func fileHasProps(file *syntax.File) bool {
	for _, decl := range file.DeclList {
		td, ok := decl.(*syntax.TypeDecl)
		if !ok {
			continue
		}
		if td.Props != nil || typeExprHasProps(td.Type) {
			return true
		}
	}
	return false
}

func typeExprHasProps(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.StructType:
		for _, f := range e.FieldList {
			if f.Props != nil || typeExprHasProps(f.Type) {
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
		return typeExprHasProps(e.X)
	case *syntax.ParenExpr:
		return typeExprHasProps(e.X)
	}
	return false
}

// ensureMetaImport makes sure file imports bolero/meta under a usable
// name and returns that name. An empty name means a dot import.
func ensureMetaImport(file *syntax.File) string {
	for _, decl := range file.DeclList {
		imp, ok := decl.(*syntax.ImportDecl)
		if !ok || imp.Path == nil {
			continue
		}
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "bolero/meta" {
			continue
		}
		if imp.LocalPkgName == nil {
			return "meta"
		}
		switch imp.LocalPkgName.Value {
		case ".":
			return ""
		case "_":
			name := freshMetaName(file)
			imp.LocalPkgName = syntax.NewName(imp.LocalPkgName.Pos(), name)
			return name
		default:
			return imp.LocalPkgName.Value
		}
	}
	pos := file.PkgName.Pos()
	name := freshMetaName(file)
	path := at(pos, &syntax.BasicLit{Value: `"bolero/meta"`, Kind: syntax.StringLit})
	imp := at(pos, &syntax.ImportDecl{
		LocalPkgName: syntax.NewName(pos, name),
		Path:         path,
	})
	file.DeclList = append([]syntax.Decl{imp}, file.DeclList...)
	return name
}

func freshMetaName(file *syntax.File) string {
	used := map[string]bool{}
	for _, decl := range file.DeclList {
		switch d := decl.(type) {
		case *syntax.ImportDecl:
			if d.LocalPkgName != nil {
				used[d.LocalPkgName.Value] = true
			}
		case *syntax.FuncDecl:
			if d.Name != nil {
				used[d.Name.Value] = true
			}
		case *syntax.TypeDecl:
			if d.Name != nil {
				used[d.Name.Value] = true
			}
		case *syntax.ConstDecl:
			for _, n := range d.NameList {
				used[n.Value] = true
			}
		case *syntax.VarDecl:
			for _, n := range d.NameList {
				used[n.Value] = true
			}
		}
	}
	for i := 0; ; i++ {
		cand := "bolero_meta"
		if i > 0 {
			cand = fmt.Sprintf("bolero_meta%d", i)
		}
		if !used[cand] {
			return cand
		}
	}
}

// patchEnumLabels replaces a generated String method's names with the
// Label property of each value, when that property is a string.
func patchEnumLabels(info *types2.Info) {
	for _, es := range enumStringLits {
		tn, _ := info.Defs[es.name].(*types2.TypeName)
		if tn == nil {
			continue
		}
		for i, lit := range es.lits {
			label, ok := types2.StringLabel(tn, es.values[i])
			if !ok {
				continue
			}
			lit.Value = strconv.Quote(label)
			tv := lit.GetTypeInfo()
			tv.Value = constant.MakeString(label)
			lit.SetTypeInfo(tv)
		}
	}
}

// fieldDescExpr writes T::F as a call to meta.NewField.
func (w *writer) fieldDescExpr(e *syntax.EnumAccessExpr) {
	tv := w.p.typeAndValue(e)
	inst, _ := types2.Unalias(tv.Type).(*types2.Named)
	if inst == nil || inst.TypeArgs() == nil || inst.TypeArgs().Len() != 2 {
		w.p.fatalf(e, "field descriptor has unexpected type %v", tv.Type)
	}
	recv := inst.TypeArgs().At(0)
	ftyp := inst.TypeArgs().At(1)
	meta := inst.Obj().Pkg()
	field, index, indirect, props := types2.DescriptorProps(recv, e.Sel.Value, w.p.curpkg)
	if field == nil {
		w.p.fatalf(e, "missing field %s", e.Sel.Value)
	}
	pos := e.Pos()
	newField := meta.Scope().Lookup("NewField").(*types2.Func)
	sig, err := types2.Instantiate(nil, newField.Type(), []types2.Type{recv, ftyp}, true)
	if err != nil {
		w.p.fatalf(e, "instantiate NewField: %v", err)
	}
	args := []syntax.Expr{
		constNode(pos, types2.Typ[types2.String], constant.MakeString(field.Name())),
		w.propsCall(pos, meta, props),
		w.getterLit(pos, recv, ftyp, field, index, indirect),
		w.setterLit(pos, recv, ftyp, field, index, indirect),
	}
	call := w.metaCall(pos, meta, newField, sig.(*types2.Signature), []types2.Type{recv, ftyp}, args)
	callTV := syntax.TypeAndValue{Type: tv.Type}
	callTV.SetIsValue()
	call.SetTypeInfo(callTV)
	w.expr(call)
}

func (w *writer) propsCall(pos syntax.Pos, meta *types2.Package, props []types2.Prop) syntax.Expr {
	newProps := meta.Scope().Lookup("NewProperties").(*types2.Func)
	propsType := meta.Scope().Lookup("Properties").(*types2.TypeName).Type()
	var args []syntax.Expr
	for _, p := range props {
		args = append(args, constNode(pos, types2.Typ[types2.String], constant.MakeString(p.Name)))
		args = append(args, constNode(pos, runtimeConstType(p.Typ), p.Val))
	}
	call := w.metaCall(pos, meta, newProps, newProps.Type().(*types2.Signature), nil, args)
	tv := syntax.TypeAndValue{Type: propsType}
	tv.SetIsValue()
	call.SetTypeInfo(tv)
	return call
}

// metaCall builds a call to fn in package meta. targs instantiates fn
// when non-nil. The call's type information is filled in by the caller
// when the result type matters; Fun's type is the (instantiated) signature.
func (w *writer) metaCall(pos syntax.Pos, meta *types2.Package, fn *types2.Func, sig *types2.Signature, targs []types2.Type, args []syntax.Expr) *syntax.CallExpr {
	fun := w.metaFun(pos, meta, fn, targs, sig)
	call := &syntax.CallExpr{Fun: fun, ArgList: args}
	call.SetPos(pos)
	return call
}

func (w *writer) metaFun(pos syntax.Pos, meta *types2.Package, fn *types2.Func, targs []types2.Type, sig *types2.Signature) syntax.Expr {
	fnName := syntax.NewName(pos, fn.Name())
	w.p.info.Uses[fnName] = fn
	var fun syntax.Expr = fnName
	if pn, pkgIdent := metaPkgName(w.p.info, meta, pos); pn != nil && pn.Name() != "." {
		w.p.info.Uses[pkgIdent] = pn
		sel := &syntax.SelectorExpr{X: pkgIdent, Sel: fnName}
		sel.SetPos(pos)
		fun = sel
	}
	if len(targs) > 0 {
		w.p.info.Instances[fnName] = types2.Instance{TypeArgs: types2.NewTypeList(targs), Type: sig}
		idx := &syntax.IndexExpr{X: fun, Index: &syntax.ListExpr{ElemList: make([]syntax.Expr, len(targs))}}
		for i := range targs {
			idx.Index.(*syntax.ListExpr).ElemList[i] = syntax.NewName(pos, "_")
		}
		idx.SetPos(pos)
		tv := syntax.TypeAndValue{Type: sig}
		tv.SetIsValue()
		idx.SetTypeInfo(tv)
		return idx
	}
	tv := syntax.TypeAndValue{Type: sig}
	tv.SetIsValue()
	switch f := fun.(type) {
	case *syntax.Name:
		f.SetTypeInfo(tv)
	case *syntax.SelectorExpr:
		f.SetTypeInfo(tv)
	}
	return fun
}

// metaPkgName returns a PkgName for meta from the package's imports and
// a fresh identifier denoting it.
func metaPkgName(info *types2.Info, meta *types2.Package, pos syntax.Pos) (*types2.PkgName, *syntax.Name) {
	for _, obj := range info.Defs {
		pn, ok := obj.(*types2.PkgName)
		if ok && pn.Imported() == meta && pn.Name() != "_" {
			return pn, syntax.NewName(pos, pn.Name())
		}
	}
	return nil, nil
}

func (w *writer) getterLit(pos syntax.Pos, recv, ftyp types2.Type, field *types2.Var, index []int, indirect bool) *syntax.FuncLit {
	sVar := types2.NewParam(pos, w.p.curpkg, "s", recv)
	res := types2.NewParam(pos, w.p.curpkg, "", ftyp)
	sig := types2.NewSignatureType(nil, nil, nil, types2.NewTuple(sVar), types2.NewTuple(res), false)
	sName := syntax.NewName(pos, "s")
	w.p.info.Uses[sName] = sVar
	sTV := syntax.TypeAndValue{Type: recv}
	sTV.SetIsValue()
	sTV.SetAddressable()
	sName.SetTypeInfo(sTV)
	sel := fieldSel(w, pos, sName, recv, field, index, indirect, false)
	ret := at(pos, &syntax.ReturnStmt{Results: sel})
	body := at(pos, &syntax.BlockStmt{List: []syntax.Stmt{ret}, Rbrace: pos})
	return funcLit(pos, sig, body)
}

func (w *writer) setterLit(pos syntax.Pos, recv, ftyp types2.Type, field *types2.Var, index []int, indirect bool) *syntax.FuncLit {
	ptr := types2.NewPointer(recv)
	sVar := types2.NewParam(pos, w.p.curpkg, "s", ptr)
	vVar := types2.NewParam(pos, w.p.curpkg, "v", ftyp)
	sig := types2.NewSignatureType(nil, nil, nil, types2.NewTuple(sVar, vVar), nil, false)
	sName := syntax.NewName(pos, "s")
	w.p.info.Uses[sName] = sVar
	sTV := syntax.TypeAndValue{Type: ptr}
	sTV.SetIsValue()
	sTV.SetAddressable()
	sName.SetTypeInfo(sTV)
	sel := fieldSel(w, pos, sName, ptr, field, index, indirect, true)
	vName := syntax.NewName(pos, "v")
	w.p.info.Uses[vName] = vVar
	vTV := syntax.TypeAndValue{Type: ftyp}
	vTV.SetIsValue()
	vName.SetTypeInfo(vTV)
	as := at(pos, &syntax.AssignStmt{Lhs: sel, Rhs: vName})
	body := at(pos, &syntax.BlockStmt{List: []syntax.Stmt{as}, Rbrace: pos})
	return funcLit(pos, sig, body)
}

func fieldSel(w *writer, pos syntax.Pos, x syntax.Expr, recv types2.Type, field *types2.Var, index []int, indirect, assign bool) *syntax.SelectorExpr {
	sel := &syntax.SelectorExpr{X: x, Sel: syntax.NewName(pos, field.Name())}
	sel.SetPos(pos)
	tv := syntax.TypeAndValue{Type: field.Type()}
	tv.SetIsValue()
	if assign {
		tv.SetAddressable()
		tv.SetAssignable()
	}
	sel.SetTypeInfo(tv)
	w.p.info.Selections[sel] = types2.NewFieldSelection(recv, field, index, indirect)
	return sel
}

func funcLit(pos syntax.Pos, sig *types2.Signature, body *syntax.BlockStmt) *syntax.FuncLit {
	lit := at(pos, &syntax.FuncLit{Body: body, Type: at(pos, &syntax.FuncType{})})
	tv := syntax.TypeAndValue{Type: sig}
	tv.SetIsValue()
	lit.SetTypeInfo(tv)
	return lit
}

func constNode(pos syntax.Pos, typ types2.Type, val constant.Value) syntax.Expr {
	lit := at(pos, &syntax.BasicLit{Kind: syntax.StringLit, Value: `""`})
	tv := syntax.TypeAndValue{Type: typ, Value: val}
	tv.SetIsValue()
	lit.SetTypeInfo(tv)
	return lit
}

// runtimeConstType is the type a property value has when passed to
// bolero/meta. Untyped constants use their default type.
func runtimeConstType(typ types2.Type) types2.Type {
	if basic, ok := types2.Unalias(typ).(*types2.Basic); ok && basic.Info()&types2.IsUntyped != 0 {
		return types2.Default(typ)
	}
	return typ
}
