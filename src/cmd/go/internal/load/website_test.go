// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package load

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindWebsitePages(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("customers/page.go", `package customers

import "bolero/ui"

const PageTitle = "Customers"
const PageOrder = 2

func Page(c *ui.Context) ui.Page { return ui.Page{} }
`)
	write("purchase-orders/page.bo", `package orders

import ui "bolero/ui"

func Page(c *ui.Context) ui.Page { return ui.Page{} }
`)
	write("customers/notes/note.go", `package notes

import . "bolero/ui"

func Page(c *Context) Page { return Page{} }
`)
	write("internal/secret/page.go", `package secret

import "bolero/ui"

func Page(c *ui.Context) ui.Page { return ui.Page{} }
`)
	write("cmd/shop/main.go", `package main

import "bolero/ui"

func Page(c *ui.Context) ui.Page { return ui.Page{} }
func main() {}
`)
	write("helper/util.go", `package helper

func Help() {}
`)

	pages, err := findWebsitePages(dir, "example.com/shop")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(pages))
	for i, p := range pages {
		got[i] = p.URL + " " + p.Title + " " + p.Import
	}
	want := []string{
		"/customers/notes Notes example.com/shop/customers/notes",
		"/purchase-orders Purchase orders example.com/shop/purchase-orders",
		"/customers Customers example.com/shop/customers",
	}
	// Order: PageOrder 0, then path. notes and purchase-orders are 0.
	// customers is 2.
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("pages:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	write("broken/bad.go", `package broken

import "bolero/ui"

func Page( { }
`)
	if _, err := findWebsitePages(dir, "example.com/shop"); err == nil {
		t.Fatal("broken page package was accepted")
	}
}
