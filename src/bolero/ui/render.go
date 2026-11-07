// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"html"
	"strings"
)

const mainID = "bolero-main"

type htmlBuilder struct {
	b strings.Builder
}

func (h *htmlBuilder) s(x string) { h.b.WriteString(x) }

func (h *htmlBuilder) text(x string) { h.b.WriteString(html.EscapeString(x)) }

func (h *htmlBuilder) attr(name, val string) {
	h.s(" ")
	h.s(name)
	h.s(`="`)
	h.text(val)
	h.s(`"`)
}

func (h *htmlBuilder) rawAttr(name, val string) {
	// val is a fixed token written by the renderer, not page data.
	h.s(" ")
	h.s(name)
	h.s(`="`)
	h.s(val)
	h.s(`"`)
}

func joinID(a, b string) string { return a + "/" + b }

func splitID(id string) (string, string, bool) {
	i := strings.IndexByte(id, '/')
	if i <= 0 || i == len(id)-1 || strings.Contains(id[i+1:], "/") {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

func (s *site) document(current, viewToken, title, mainInner string) string {
	var h htmlBuilder
	h.s("<!DOCTYPE html>\n<html lang=\"en\"><head>")
	h.s(`<meta charset="utf-8">`)
	h.s(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	h.s("<title>")
	h.text(title)
	h.s("</title>")
	h.s(`<link rel="stylesheet" href="/bolero/static/main.css">`)
	h.s(`<script defer src="/bolero/static/client.js"></script>`)
	h.s("</head><body")
	if viewToken != "" {
		h.attr("data-bolero-view", viewToken)
	}
	h.s(">")
	s.nav(&h, current)
	h.s(`<main data-bolero-id="`)
	h.s(mainID)
	h.s(`">`)
	h.s(mainInner)
	h.s("</main></body></html>")
	return h.b.String()
}

func (s *site) nav(h *htmlBuilder, current string) {
	h.s("<bolero-nav><header>")
	name := s.name
	if name == "" {
		name = "Bolero"
	}
	h.text(name)
	h.s("</header>")
	for _, r := range s.routes {
		h.s("<a")
		h.attr("href", r.Path)
		if r.Path == current {
			h.rawAttr("class", "current")
		}
		h.s(">")
		label := r.Title
		if label == "" {
			label = r.Path
		}
		h.text(label)
		h.s("</a>")
	}
	h.s("</bolero-nav>")
}

func renderMain(title, body string) string {
	var h htmlBuilder
	h.s(`<main data-bolero-id="`)
	h.s(mainID)
	h.s(`"><header>`)
	h.text(title)
	h.s("</header>")
	h.s(body)
	h.s("</main>")
	return h.b.String()
}

func renderPageBody(p *Page) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	var h htmlBuilder
	for gi := range p.Groups {
		renderGroup(&h, &p.Groups[gi])
	}
	return h.b.String(), nil
}

func renderGroup(h *htmlBuilder, g *Group) {
	h.s("<bolero-group>")
	if g.Collapsible {
		h.s("<details open><summary>")
		h.text(g.Title)
		renderTooltip(h, g.Description)
		h.s("</summary>")
	} else {
		h.s("<header>")
		h.text(g.Title)
		renderTooltip(h, g.Description)
		h.s("</header>")
	}
	h.s(`<div class="widgets">`)
	for _, item := range g.Items {
		switch w := item.(type) {
		case Form:
			renderForm(h, &w)
		case *Form:
			if w != nil {
				renderForm(h, w)
			}
		case Table:
			renderTable(h, &w)
		case *Table:
			if w != nil {
				renderTable(h, w)
			}
		case ActionPicker:
			renderActions(h, &w)
		case *ActionPicker:
			if w != nil {
				renderActions(h, w)
			}
		}
	}
	h.s("</div>")
	if g.Collapsible {
		h.s("</details>")
	}
	h.s("</bolero-group>")
}

func renderTooltip(h *htmlBuilder, text string) {
	if text == "" {
		return
	}
	h.s(`<bolero-tooltip role="tooltip">`)
	h.text(text)
	h.s("</bolero-tooltip>")
}

func renderForm(h *htmlBuilder, f *Form) {
	h.s("<form")
	h.attr("data-bolero-id", f.Name)
	h.s(">")
	if f.Title != "" {
		h.s("<header>")
		h.text(f.Title)
		h.s("</header>")
	}
	h.s(`<div class="fields">`)
	for gi := range f.FieldGroups {
		g := &f.FieldGroups[gi]
		h.s("<bolero-column>")
		if g.Title != "" {
			h.s("<header>")
			h.text(g.Title)
			h.s("</header>")
		}
		for fi := range g.Fields {
			renderField(h, f.Name, &g.Fields[fi])
		}
		h.s("</bolero-column>")
	}
	h.s("</div></form>")
}

func renderField(h *htmlBuilder, form string, f *Field) {
	id := joinID(form, f.Name)
	h.s(`<div class="field"`)
	h.attr("data-bolero-id", id)
	h.s(">")
	h.s(`<div class="labelRow"><label`)
	h.attr("for", id)
	h.s(">")
	label := f.Label
	if label == "" {
		label = f.Name
	}
	h.text(label)
	h.s("</label>")
	if f.Required {
		h.s(`<span class="required">∗</span>`)
	}
	renderTooltip(h, f.Description)
	h.s("</div>")
	renderControl(h, id, f)
	if f.Error != "" {
		h.s(`<div class="error-message">`)
		h.text(f.Error)
		h.s("</div>")
	}
	h.s("</div>")
}

func renderControl(h *htmlBuilder, id string, f *Field) {
	switch f.Kind {
	case Boolean:
		h.s("<input")
		h.rawAttr("type", "checkbox")
		h.attr("id", id)
		h.attr("data-bolero-send", id)
		h.rawAttr("data-bolero-event", "change")
		h.rawAttr("value", "true")
		if f.Value == "true" {
			h.s(" checked")
		}
		if f.ReadOnly {
			h.s(" disabled")
		}
		if f.Error != "" {
			h.rawAttr("class", "error")
		}
		h.s(">")
	case Selection:
		h.s("<select")
		h.attr("id", id)
		h.attr("data-bolero-send", id)
		h.rawAttr("data-bolero-event", "change")
		if f.ReadOnly {
			h.s(" disabled")
		}
		if f.Error != "" {
			h.rawAttr("class", "error")
		}
		h.s(">")
		h.s(`<option value=""></option>`)
		for _, o := range f.Options {
			h.s("<option")
			h.attr("value", o.Value)
			if o.Value == f.Value && f.Value != "" {
				h.s(" selected")
			}
			h.s(">")
			label := o.Label
			if label == "" {
				label = o.Value
			}
			h.text(label)
			h.s("</option>")
		}
		h.s("</select>")
	default:
		h.s("<input")
		h.rawAttr("type", inputType(f.Kind))
		h.attr("id", id)
		h.attr("data-bolero-send", id)
		h.rawAttr("data-bolero-event", "change")
		h.attr("value", f.Value)
		if f.Kind == Text {
			h.rawAttr("autocomplete", "off")
		}
		if f.Kind == Decimal {
			h.rawAttr("step", "any")
		}
		if f.Kind == Integer {
			h.rawAttr("step", "1")
		}
		if f.Min != "" {
			h.attr("min", f.Min)
		}
		if f.Max != "" {
			h.attr("max", f.Max)
		}
		if f.ReadOnly {
			h.s(" readonly")
		}
		if f.Error != "" {
			h.rawAttr("class", "error")
		}
		h.s(">")
	}
}

func inputType(k Kind) string {
	switch k {
	case Integer, Decimal:
		return "number"
	case Date:
		return "date"
	case Time:
		return "time"
	case DateTime:
		return "datetime-local"
	default:
		return "text"
	}
}

func renderTable(h *htmlBuilder, t *Table) {
	h.s(`<div class="table"`)
	h.attr("data-bolero-id", t.Name)
	h.s(">")
	if t.Title != "" {
		h.s("<header>")
		h.text(t.Title)
		h.s("</header>")
	}
	cols := t.columns()
	h.s("<table><thead>")
	grouped := false
	for _, g := range t.FieldGroups {
		if g.Title != "" {
			grouped = true
			break
		}
	}
	if grouped {
		h.s("<tr>")
		for _, g := range t.FieldGroups {
			h.s("<th")
			h.s(` colspan="`)
			h.s(itoa(len(g.Fields)))
			h.s(`">`)
			h.text(g.Title)
			h.s("</th>")
		}
		h.s("</tr>")
	}
	h.s("<tr>")
	for _, col := range cols {
		h.s("<th><button type=\"button\"")
		h.attr("data-bolero-send", joinID(t.Name, col.Name))
		h.rawAttr("data-bolero-event", "sort")
		h.s(">")
		label := col.Label
		if label == "" {
			label = col.Name
		}
		h.text(label)
		if t.sortBy == col.Name {
			if t.sortDesc {
				h.s(" ▼")
			} else {
				h.s(" ▲")
			}
		}
		h.s("</button></th>")
	}
	h.s("</tr></thead><tbody>")
	for _, row := range t.order() {
		h.s("<tr")
		if t.OnRow != nil {
			h.attr("data-bolero-send", t.Name)
			h.rawAttr("data-bolero-event", "row")
			h.attr("data-bolero-value", itoa(row))
		}
		h.s(">")
		for c := range cols {
			h.s("<td>")
			h.text(cell(t.Rows, row, c))
			h.s("</td>")
		}
		h.s("</tr>")
	}
	h.s("</tbody></table></div>")
}

func renderActions(h *htmlBuilder, a *ActionPicker) {
	h.s(`<div class="actions"`)
	h.attr("data-bolero-id", a.Name)
	h.s(">")
	if a.Title != "" {
		h.s("<header>")
		h.text(a.Title)
		h.s("</header>")
	}
	for _, act := range a.Actions {
		h.s(`<button type="button"`)
		h.attr("data-bolero-send", joinID(a.Name, act.Name))
		h.rawAttr("data-bolero-event", "click")
		h.s(">")
		label := act.Label
		if label == "" {
			label = act.Name
		}
		h.text(label)
		h.s("</button>")
	}
	h.s("</div>")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func notice(text string) string {
	return `<p class="notice">` + html.EscapeString(text) + `</p>`
}

func renderFieldHTML(form string, f *Field) string {
	var h htmlBuilder
	renderField(&h, form, f)
	return h.b.String()
}

func renderTableHTML(t *Table) string {
	var h htmlBuilder
	renderTable(&h, t)
	return h.b.String()
}
