// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgbits

import (
	"go/constant"
	"slices"
)

// BoleroRootIdx is the index, within SectionMeta, of the optional Bolero
// root. It records which objects of the bitstream are declared in Bolero
// source files, and which methods of its types are. Decoders that don't
// know about Bolero never look at it, so the bitstream version and format
// stay those of upstream Go.
const BoleroRootIdx RelElemIdx = 2

// BoleroObj describes the Bolero declarations of one object.
type BoleroObj struct {
	Decl    bool     // the object is declared in a Bolero source file
	Methods []string // names of the object's methods declared in Bolero source files
	Enum    []string // if the object declares an enum type, the names of its values
	Props   []BoleroProp
}

// BoleroProp is one static property of a type, field or enum value.
// On is 't' for the type, 'f' for a field named Name, or 'v' for an
// enum value named Name. Basic >= 0 is the constant's basic kind;
// otherwise the constant's type is the named type TypePkg.TypeName.
type BoleroProp struct {
	On       byte
	Name     string
	Key      string
	Val      constant.Value
	Basic    int
	TypePkg  string
	TypeName string
}

// WriteBoleroRoot writes the Bolero root, which describes the objects in
// SectionObj with the given indices. It must be called after the public and
// private roots are created, and only if objs is not empty.
func (pw *PkgEncoder) WriteBoleroRoot(objs map[RelElemIdx]BoleroObj) {
	w := pw.NewEncoderRaw(SectionMeta)
	assert(w.Idx == BoleroRootIdx)

	idxs := make([]RelElemIdx, 0, len(objs))
	for idx := range objs {
		idxs = append(idxs, idx)
	}
	slices.Sort(idxs)

	w.Len(len(idxs))
	for _, idx := range idxs {
		obj := objs[idx]
		w.Reloc(SectionObj, idx)
		w.Bool(obj.Decl)
		w.Len(len(obj.Methods))
		for _, name := range obj.Methods {
			w.String(name)
		}
		w.Len(len(obj.Enum))
		for _, name := range obj.Enum {
			w.String(name)
		}
		w.Len(len(obj.Props))
		for _, p := range obj.Props {
			w.Uint64(uint64(p.On))
			w.String(p.Name)
			w.String(p.Key)
			w.Value(p.Val)
			if w.Bool(p.Basic >= 0) {
				w.Len(p.Basic)
			} else {
				w.String(p.TypePkg)
				w.String(p.TypeName)
			}
		}
	}
	w.Flush()
}

// BoleroRoot returns the contents of the Bolero root, keyed by index in
// SectionObj. It returns nil if the bitstream declares nothing in Bolero.
func (pr *PkgDecoder) BoleroRoot() map[RelElemIdx]BoleroObj {
	if pr.NumElems(SectionMeta) <= int(BoleroRootIdx) {
		return nil
	}
	r := pr.NewDecoderRaw(SectionMeta, BoleroRootIdx)
	objs := make(map[RelElemIdx]BoleroObj)
	for range r.Len() {
		idx := r.Reloc(SectionObj)
		obj := BoleroObj{Decl: r.Bool()}
		for range r.Len() {
			obj.Methods = append(obj.Methods, r.String())
		}
		for range r.Len() {
			obj.Enum = append(obj.Enum, r.String())
		}
		for range r.Len() {
			p := BoleroProp{On: byte(r.Uint64())}
			p.Name = r.String()
			p.Key = r.String()
			p.Val = r.Value()
			if r.Bool() {
				p.Basic = r.Len()
			} else {
				p.Basic = -1
				p.TypePkg = r.String()
				p.TypeName = r.String()
			}
			obj.Props = append(obj.Props, p)
		}
		objs[idx] = obj
	}
	return objs
}
