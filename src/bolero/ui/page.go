// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ui serves a Bolero application.
//
// A page is the value returned by a package's Page function. The bolero command
// turns each such package into a URL: the directory customers/invoices is
// served at /customers/invoices. The server renders the page to HTML.
// Edits arrive over a WebSocket and update the page value kept for that
// browser tab.
package ui

import (
	"fmt"
	"path"
	"strings"
	"sync"
)

// Route is one page. Path is the URL path, such as "/customers".
// Title is the navigation label. Order sorts the navigation;
// equal orders are sorted by Path. Page builds the page value.
type Route struct {
	Path  string
	Title string
	Order int
	Page  func(*Context) Page
}

// Page is one screen. Groups are stacked in order.
type Page struct {
	Title  string
	Groups []Group
}

// Group is a titled section of a page.
// Items are stacked in order. Description is a tooltip.
// A collapsible group can be folded by the browser and starts open.
type Group struct {
	Title       string
	Description string
	Collapsible bool
	Items       []Elem
}

// Elem is a widget a group can hold.
// The implementations are Form, Table, ActionPicker, and pointers to those.
type Elem interface {
	elem()
}

type (
	regState struct {
		mu      sync.Mutex
		routes  []Route
		appName string
	}
)

var registry regState

// Register adds a page. It panics if Path is not a clean URL path other
// than "/", or if Page is nil, or if Path is already registered.
// An app.json module does not call Register: the bolero command does.
func Register(r Route) {
	if r.Page == nil || !validPath(r.Path) {
		panic("bolero/ui: invalid route")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, old := range registry.routes {
		if old.Path == r.Path {
			panic("bolero/ui: route " + r.Path + " already registered")
		}
	}
	registry.routes = append(registry.routes, r)
}

// SetAppName sets the name shown in the navigation.
// The last call wins. An empty name is ignored.
func SetAppName(name string) {
	if name == "" {
		return
	}
	registry.mu.Lock()
	registry.appName = name
	registry.mu.Unlock()
}

func snapshot() (string, []Route) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	out := make([]Route, len(registry.routes))
	copy(out, registry.routes)
	return registry.appName, out
}

func validPath(p string) bool {
	if p == "" || p[0] != '/' || strings.HasSuffix(p, "/") {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	return !strings.Contains(p, "//")
}

func (p *Page) validate() error {
	seen := map[string]string{}
	for gi := range p.Groups {
		for _, item := range p.Groups[gi].Items {
			switch w := item.(type) {
			case Form:
				if err := note(seen, "form", w.Name); err != nil {
					return err
				}
				if err := validateFields(seen, "form "+w.Name, w.FieldGroups); err != nil {
					return err
				}
			case *Form:
				if w == nil {
					return fmt.Errorf("nil form")
				}
				if err := note(seen, "form", w.Name); err != nil {
					return err
				}
				if err := validateFields(seen, "form "+w.Name, w.FieldGroups); err != nil {
					return err
				}
			case Table:
				if err := note(seen, "table", w.Name); err != nil {
					return err
				}
				if err := validateFields(seen, "table "+w.Name, w.FieldGroups); err != nil {
					return err
				}
			case *Table:
				if w == nil {
					return fmt.Errorf("nil table")
				}
				if err := note(seen, "table", w.Name); err != nil {
					return err
				}
				if err := validateFields(seen, "table "+w.Name, w.FieldGroups); err != nil {
					return err
				}
			case ActionPicker:
				if err := note(seen, "action picker", w.Name); err != nil {
					return err
				}
				if err := validateActions(seen, w.Name, w.Actions); err != nil {
					return err
				}
			case *ActionPicker:
				if w == nil {
					return fmt.Errorf("nil action picker")
				}
				if err := note(seen, "action picker", w.Name); err != nil {
					return err
				}
				if err := validateActions(seen, w.Name, w.Actions); err != nil {
					return err
				}
			default:
				if item == nil {
					return fmt.Errorf("nil widget")
				}
				return fmt.Errorf("unknown widget")
			}
		}
	}
	return nil
}

func note(seen map[string]string, kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s has no name", kind)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("%s name contains /", kind)
	}
	if prev, ok := seen[name]; ok {
		return fmt.Errorf("two widgets named %s (%s and %s)", name, prev, kind)
	}
	seen[name] = kind
	return nil
}

func validateFields(seen map[string]string, where string, groups []FieldGroup) error {
	names := map[string]bool{}
	for _, g := range groups {
		for _, f := range g.Fields {
			if f.Name == "" {
				return fmt.Errorf("field has no name in %s", where)
			}
			if strings.Contains(f.Name, "/") {
				return fmt.Errorf("field name %s contains /", f.Name)
			}
			if names[f.Name] {
				return fmt.Errorf("two fields named %s in %s", f.Name, where)
			}
			names[f.Name] = true
		}
	}
	return nil
}

func validateActions(seen map[string]string, picker string, actions []Action) error {
	names := map[string]bool{}
	for _, a := range actions {
		if a.Name == "" {
			return fmt.Errorf("action has no name in %s", picker)
		}
		if strings.Contains(a.Name, "/") {
			return fmt.Errorf("action name %s contains /", a.Name)
		}
		if names[a.Name] {
			return fmt.Errorf("two actions named %s in %s", a.Name, picker)
		}
		names[a.Name] = true
	}
	return nil
}

// editField applies fn to the named field of the named form.
// A form stored by value is written back.
func (p *Page) editField(form, name string, fn func(*Field)) bool {
	for gi := range p.Groups {
		items := p.Groups[gi].Items
		for ii := range items {
			switch f := items[ii].(type) {
			case *Form:
				if f != nil && f.Name == form {
					if field := fieldPtr(f, name); field != nil {
						fn(field)
						return true
					}
				}
			case Form:
				if f.Name == form {
					if field := fieldPtr(&f, name); field != nil {
						fn(field)
						p.Groups[gi].Items[ii] = f
						return true
					}
				}
			}
		}
	}
	return false
}

func (p *Page) readField(form, name string) (Field, bool) {
	var out Field
	var ok bool
	p.editField(form, name, func(f *Field) {
		out = *f
		ok = true
	})
	return out, ok
}

func (p *Page) form(name string) (Form, bool) {
	for gi := range p.Groups {
		for _, item := range p.Groups[gi].Items {
			switch f := item.(type) {
			case *Form:
				if f != nil && f.Name == name {
					return *f, true
				}
			case Form:
				if f.Name == name {
					return f, true
				}
			}
		}
	}
	return Form{}, false
}

func (p *Page) editTable(name string, fn func(*Table)) bool {
	for gi := range p.Groups {
		items := p.Groups[gi].Items
		for ii := range items {
			switch t := items[ii].(type) {
			case *Table:
				if t != nil && t.Name == name {
					fn(t)
					return true
				}
			case Table:
				if t.Name == name {
					fn(&t)
					p.Groups[gi].Items[ii] = t
					return true
				}
			}
		}
	}
	return false
}

func fieldPtr(f *Form, name string) *Field {
	for gi := range f.FieldGroups {
		fields := f.FieldGroups[gi].Fields
		for fi := range fields {
			if fields[fi].Name == name {
				return &f.FieldGroups[gi].Fields[fi]
			}
		}
	}
	return nil
}

func resetForTest() {
	registry.mu.Lock()
	registry.routes = nil
	registry.appName = ""
	registry.mu.Unlock()
	sessionsMu.Lock()
	sessions = map[string]*session{}
	sessionsMu.Unlock()
}
