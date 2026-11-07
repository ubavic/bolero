// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import "net/url"

// Context is the handle passed to Page and to action handlers.
// Page is called once for a page load. Handlers of that page value
// run later, on the same context.
//
// Input, SetInput, ClearInput, and FieldError address a field by its
// form name and field name. During Page, the page value is not stored
// yet, so Input reports that the field is absent.
type Context struct {
	view *view
}

// Path returns the page path, such as "/customers".
func (c *Context) Path() string {
	if c == nil || c.view == nil {
		return ""
	}
	return c.view.path
}

// Query returns the query from the request that opened this view.
func (c *Context) Query() url.Values {
	if c == nil || c.view == nil || c.view.query == nil {
		return url.Values{}
	}
	return c.view.query
}

// Input returns the current value of a field.
// The boolean is false when the form or the field does not exist.
func (c *Context) Input(form, field string) (string, bool) {
	if c == nil || c.view == nil {
		return "", false
	}
	f, ok := c.view.page.readField(form, field)
	if !ok {
		return "", false
	}
	return f.Value, true
}

// SetInput sets the current value of a field and clears its error.
// It does nothing when the field does not exist.
func (c *Context) SetInput(form, field, value string) {
	if c == nil || c.view == nil {
		return
	}
	c.view.page.editField(form, field, func(f *Field) {
		f.Value = value
		f.Error = ""
	})
}

// ClearInput sets the field value and its error to empty.
func (c *Context) ClearInput(form, field string) {
	c.SetInput(form, field, "")
}

// FieldError returns the field's current error, or "" if there is none
// or the field does not exist.
func (c *Context) FieldError(form, field string) string {
	if c == nil || c.view == nil {
		return ""
	}
	f, ok := c.view.page.readField(form, field)
	if !ok {
		return ""
	}
	return f.Error
}

// Valid reports whether every field of the named form has an empty error
// after a fresh check. It reports false when the form does not exist.
func (c *Context) Valid(form string) bool {
	if c == nil || c.view == nil {
		return false
	}
	f, ok := c.view.page.applyChecks(form)
	if !ok {
		return false
	}
	good := true
	f.eachField(func(field *Field) {
		if field.Error != "" {
			good = false
		}
	})
	return good
}
