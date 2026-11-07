// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"math/big"
	"strconv"
	"time"
)

func (Form) elem() {}

// Form is a set of fields the user can edit.
// Name is unique on the page and is part of each field's event id.
type Form struct {
	Name        string
	Title       string
	FieldGroups []FieldGroup
}

// FieldGroup is a labeled cluster of fields.
// In a form the groups of one form sit in a row.
// In a table a group is a column group.
type FieldGroup struct {
	Title  string
	Fields []Field
}

// Field is one input, or one table column.
// Value is the current text. The zero Kind is text.
// Error is shown under the input. The server sets it when the value is rejected.
// Validate, when set, runs after the kind check. A nil error clears the field error.
type Field struct {
	Name        string
	Label       string
	Kind        Kind
	Value       string
	Required    bool
	ReadOnly    bool
	Description string
	Options     []Option
	Min         string
	Max         string
	Error       string
	Validate    func(value string) error
}

// Option is one choice of a selection field.
type Option struct {
	Value string
	Label string
}

// Kind is the kind of value a field accepts.
type Kind int

const (
	Text Kind = iota
	Integer
	Decimal
	Selection
	Date
	Time
	DateTime
	Boolean
)

// check sets f.Error from Required, Kind, Min, Max, and Validate.
// An empty value is accepted when the field is not required.
// A boolean is empty unless it is "true" or "false", and a required
// boolean must be "true".
func (f *Field) check() {
	f.Error = ""
	if f.Kind == Boolean {
		if f.Value != "" && f.Value != "true" && f.Value != "false" {
			f.Error = "not a boolean"
			return
		}
		if f.Required && f.Value != "true" {
			f.Error = "required"
		}
		return
	}
	if f.Value == "" {
		if f.Required {
			f.Error = "required"
		}
		return
	}
	if msg := kindError(f); msg != "" {
		f.Error = msg
		return
	}
	if f.Validate != nil {
		if err := f.Validate(f.Value); err != nil {
			f.Error = err.Error()
		}
	}
}

func kindError(f *Field) string {
	switch f.Kind {
	case Text:
		return ""
	case Integer:
		n, err := strconv.ParseInt(f.Value, 10, 64)
		if err != nil {
			return "not an integer"
		}
		if msg := bounds(f, func(s string) (int, bool) {
			m, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return 0, false
			}
			return cmp64(n, m), true
		}); msg != "" {
			return msg
		}
	case Decimal:
		n, ok := parseDecimal(f.Value)
		if !ok {
			return "not a decimal number"
		}
		if msg := bounds(f, func(s string) (int, bool) {
			m, ok := parseDecimal(s)
			if !ok {
				return 0, false
			}
			return n.Cmp(m), true
		}); msg != "" {
			return msg
		}
	case Selection:
		for _, o := range f.Options {
			if o.Value == f.Value {
				return ""
			}
		}
		return "not one of the choices"
	case Date:
		got, err := time.Parse("2006-01-02", f.Value)
		if err != nil {
			return "not a date"
		}
		if msg := timeBounds(f, got, "2006-01-02"); msg != "" {
			return msg
		}
	case Time:
		got, err := time.Parse("15:04", f.Value)
		if err != nil {
			return "not a time"
		}
		if msg := timeBounds(f, got, "15:04"); msg != "" {
			return msg
		}
	case DateTime:
		got, err := time.Parse("2006-01-02T15:04", f.Value)
		if err != nil {
			return "not a date and time"
		}
		if msg := timeBounds(f, got, "2006-01-02T15:04"); msg != "" {
			return msg
		}
	case Boolean:
		if f.Value != "true" && f.Value != "false" {
			return "not a boolean"
		}
	default:
		return "unknown field kind"
	}
	return ""
}

func bounds(f *Field, cmp func(string) (int, bool)) string {
	if f.Min != "" {
		c, ok := cmp(f.Min)
		if !ok {
			return "invalid minimum"
		}
		if c < 0 {
			return "below the minimum"
		}
	}
	if f.Max != "" {
		c, ok := cmp(f.Max)
		if !ok {
			return "invalid maximum"
		}
		if c > 0 {
			return "above the maximum"
		}
	}
	return ""
}

func timeBounds(f *Field, got time.Time, layout string) string {
	return bounds(f, func(s string) (int, bool) {
		t, err := time.Parse(layout, s)
		if err != nil {
			return 0, false
		}
		return got.Compare(t), true
	})
}

func cmp64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func parseDecimal(s string) (*big.Rat, bool) {
	if s == "" {
		return nil, false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	if i >= len(s) {
		return nil, false
	}
	dot := false
	digits := 0
	for ; i < len(s); i++ {
		switch {
		case s[i] == '.':
			if dot {
				return nil, false
			}
			dot = true
		case s[i] >= '0' && s[i] <= '9':
			digits++
		default:
			return nil, false
		}
	}
	if digits == 0 {
		return nil, false
	}
	n := new(big.Rat)
	if _, ok := n.SetString(s); !ok {
		return nil, false
	}
	return n, true
}

func (f *Form) eachField(fn func(*Field)) {
	for gi := range f.FieldGroups {
		for fi := range f.FieldGroups[gi].Fields {
			fn(&f.FieldGroups[gi].Fields[fi])
		}
	}
}

// applyChecks validates every field of the form stored in the page.
// The stored form is updated, including one that was stored by value.
func (p *Page) applyChecks(name string) (Form, bool) {
	found := false
	var out Form
	for gi := range p.Groups {
		items := p.Groups[gi].Items
		for ii := range items {
			switch f := items[ii].(type) {
			case *Form:
				if f != nil && f.Name == name {
					f.eachField(func(field *Field) { field.check() })
					return *f, true
				}
			case Form:
				if f.Name == name {
					f.eachField(func(field *Field) { field.check() })
					p.Groups[gi].Items[ii] = f
					out = f
					found = true
				}
			}
		}
	}
	return out, found
}
