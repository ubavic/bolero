// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import "strings"

func (Table) elem() {}

// Table shows rows of text.
// Each field is a column, and each field group is a column group.
// Rows align with the fields from left to right. A short row is blank
// in the missing columns.
//
// A header click sorts the rows as text. The sort is a display order;
// Rows itself is left as the page set it. OnRow receives the index in
// Rows of the clicked row, which is not the index on screen when the
// table is sorted.
type Table struct {
	Name        string
	Title       string
	FieldGroups []FieldGroup
	Rows        [][]string
	OnRow       func(c *Context, row int)

	sortBy   string
	sortDesc bool
}

func (t *Table) columns() []Field {
	var cols []Field
	for _, g := range t.FieldGroups {
		cols = append(cols, g.Fields...)
	}
	return cols
}

func (t *Table) order() []int {
	idx := make([]int, len(t.Rows))
	for i := range idx {
		idx[i] = i
	}
	col := -1
	cols := t.columns()
	for i, f := range cols {
		if f.Name == t.sortBy {
			col = i
			break
		}
	}
	if col < 0 {
		return idx
	}
	// Insertion sort keeps the implementation small and stable.
	for i := 1; i < len(idx); i++ {
		j := i
		for j > 0 && t.less(idx[j], idx[j-1], col) {
			idx[j], idx[j-1] = idx[j-1], idx[j]
			j--
		}
	}
	return idx
}

func (t *Table) less(a, b, col int) bool {
	va := cell(t.Rows, a, col)
	vb := cell(t.Rows, b, col)
	c := strings.Compare(va, vb)
	if t.sortDesc {
		return c > 0
	}
	return c < 0
}

func cell(rows [][]string, row, col int) string {
	if row < 0 || row >= len(rows) || col < 0 || col >= len(rows[row]) {
		return ""
	}
	return rows[row][col]
}

func (t *Table) toggleSort(name string) {
	if t.sortBy == name {
		t.sortDesc = !t.sortDesc
		return
	}
	t.sortBy = name
	t.sortDesc = false
}
