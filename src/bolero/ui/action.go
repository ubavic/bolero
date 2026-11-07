// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

func (ActionPicker) elem() {}

// ActionPicker is a row of buttons.
// Name is unique on the page.
type ActionPicker struct {
	Name    string
	Title   string
	Actions []Action
}

// Action is one button.
// Form, when set, names a form on the page. OnClick runs only when
// every field of that form is valid. OnClick may be nil.
type Action struct {
	Name    string
	Label   string
	Form    string
	OnClick func(c *Context)
}

func (p *Page) action(picker, name string) (Action, bool) {
	for gi := range p.Groups {
		for _, item := range p.Groups[gi].Items {
			var ap ActionPicker
			switch w := item.(type) {
			case *ActionPicker:
				if w == nil || w.Name != picker {
					continue
				}
				ap = *w
			case ActionPicker:
				if w.Name != picker {
					continue
				}
				ap = w
			default:
				continue
			}
			for _, a := range ap.Actions {
				if a.Name == name {
					return a, true
				}
			}
		}
	}
	return Action{}, false
}
