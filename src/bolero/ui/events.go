// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
)

// handle runs one event against the view. The caller holds v.mu.
// ok is false when the event does not match a widget.
func (v *view) handle(msg wsIn) (id, fragment string, ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("event %s: %v\n%s", msg.Event, rec, debug.Stack())
			id = mainID
			fragment = `<main data-bolero-id="` + mainID + `">` + headingInner("Error", notice(fmt.Sprint(rec))) + `</main>`
			ok = true
		}
	}()
	switch msg.Event {
	case "change":
		return v.onChange(msg.ID, msg.Value)
	case "click":
		return v.onClick(msg.ID)
	case "sort":
		return v.onSort(msg.ID)
	case "row":
		return v.onRow(msg.ID, msg.Value)
	default:
		return "", "", false
	}
}

func (v *view) onChange(id, value string) (string, string, bool) {
	form, name, ok := splitID(id)
	if !ok {
		return "", "", false
	}
	f, exists := v.page.readField(form, name)
	if !exists {
		return "", "", false
	}
	if f.ReadOnly {
		return id, renderFieldHTML(form, &f), true
	}
	v.page.editField(form, name, func(field *Field) {
		field.Value = value
		field.check()
	})
	f, _ = v.page.readField(form, name)
	return id, renderFieldHTML(form, &f), true
}

func (v *view) onClick(id string) (string, string, bool) {
	picker, name, ok := splitID(id)
	if !ok {
		return "", "", false
	}
	act, exists := v.page.action(picker, name)
	if !exists {
		return "", "", false
	}
	ctx := &Context{view: v}
	if act.Form != "" {
		if _, found := v.page.form(act.Form); !found {
			log.Printf("action %s: no form %s", name, act.Form)
			return mainID, v.mainElement(), true
		}
		if !ctx.Valid(act.Form) {
			return mainID, v.mainElement(), true
		}
	}
	if act.OnClick != nil {
		act.OnClick(ctx)
	}
	return mainID, v.mainElement(), true
}

func (v *view) onSort(id string) (string, string, bool) {
	table, col, ok := splitID(id)
	if !ok {
		return "", "", false
	}
	changed := false
	found := v.page.editTable(table, func(t *Table) {
		for _, c := range t.columns() {
			if c.Name == col {
				t.toggleSort(col)
				changed = true
				return
			}
		}
	})
	if !found || !changed {
		return "", "", false
	}
	var fragment string
	v.page.editTable(table, func(t *Table) {
		fragment = renderTableHTML(t)
	})
	return table, fragment, true
}

func (v *view) onRow(id, value string) (string, string, bool) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return "", "", false
	}
	var handler func(*Context, int)
	found := v.page.editTable(id, func(t *Table) {
		if t.OnRow == nil || n < 0 || n >= len(t.Rows) {
			return
		}
		handler = t.OnRow
	})
	if !found || handler == nil {
		return "", "", false
	}
	handler(&Context{view: v}, n)
	return mainID, v.mainElement(), true
}
