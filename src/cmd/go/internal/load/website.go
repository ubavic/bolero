// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package load

import (
	"cmd/go/internal/cache"
	"cmd/go/internal/cfg"
	"cmd/go/internal/modload"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// websitePage is one page package discovered from the module layout.
type websitePage struct {
	Import string
	URL    string
	Title  string
	Order  int
}

// websitePlan is the page list registered into one main package.
type websitePlan struct {
	pages   []websitePage
	appName string
}

// attachWebsite adds the generated page registry to a main package of an
// app.json module. Imports are appended to p.Imports so the caller can
// load them with the rest of the package's imports.
func attachWebsite(loaderstate *modload.State, ctx context.Context, p *Package) error {
	if p.Name != "main" || p.Internal.CmdlineFiles || p.Standard || !cfg.ModulesEnabled {
		return nil
	}
	info := modload.PackageModuleInfo(loaderstate, ctx, p.ImportPath)
	if info == nil || !info.Main || !modload.IsAppJSON(info.GoMod) || info.Dir == "" {
		return nil
	}
	pages, err := findWebsitePages(info.Dir, info.Path)
	if err != nil {
		return err
	}
	plan := websitePlan{pages: pages, appName: appDisplayName(info.Dir, info.Path)}
	if len(plan.pages) == 0 {
		return nil
	}
	for _, page := range plan.pages {
		if err := modload.LoadExtraPackage(loaderstate, ctx, page.Import); err != nil {
			return err
		}
	}
	src, err := renderWebsiteFile(p.Name, plan)
	if err != nil {
		return err
	}
	file, err := writeWebsiteFile(src)
	if err != nil {
		return err
	}
	p.GoFiles = append(p.GoFiles, file)
	// Imports past RawImports are paired with CompiledImports.
	// bolero list indexes that pairing, so the generated imports are recorded
	// there too. They are not cgo imports; the slice is the extra-import list.
	addImport := func(path string) {
		for _, old := range p.Imports {
			if old == path {
				return
			}
		}
		p.Imports = append(p.Imports, path)
		p.Internal.CompiledImports = append(p.Internal.CompiledImports, path)
	}
	addImport("bolero/ui")
	for _, page := range plan.pages {
		addImport(page.Import)
	}
	return nil
}

// findWebsitePages walks the module and returns a page for every package
// that declares func Page(*ui.Context) ui.Page. Package main, the module
// root, and internal, testdata, vendor, and dot or underscore directories
// are skipped.
func findWebsitePages(modRoot, modPath string) ([]websitePage, error) {
	var pages []websitePage
	err := filepath.WalkDir(modRoot, func(dir string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if dir != modRoot && (skipWebsiteDir(d.Name()) || nestedModule(dir)) {
			return fs.SkipDir
		}
		page, ok, err := websitePageIn(dir, modRoot, modPath)
		if err != nil {
			return err
		}
		if ok {
			pages = append(pages, page)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(pages, func(a, b websitePage) int {
		if a.Order != b.Order {
			return a.Order - b.Order
		}
		return strings.Compare(a.URL, b.URL)
	})
	return pages, nil
}

func skipWebsiteDir(name string) bool {
	return name == "internal" || name == "testdata" || name == "vendor" ||
		strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func nestedModule(dir string) bool {
	for _, name := range []string{"go.mod", "app.json"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func websitePageIn(dir, modRoot, modPath string) (websitePage, bool, error) {
	rel, err := filepath.Rel(modRoot, dir)
	if err != nil {
		return websitePage{}, false, err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "..") {
		return websitePage{}, false, nil
	}
	bc := cfg.BuildContext
	bc.GOPATH = ""
	bp, err := bc.ImportDir(dir, 0)
	if err != nil {
		if _, ok := err.(*build.NoGoError); ok {
			return websitePage{}, false, nil
		}
		return websitePage{}, false, err
	}
	if bp.Name == "" || bp.Name == "main" || len(bp.GoFiles) == 0 {
		return websitePage{}, false, nil
	}
	fset := token.NewFileSet()
	found := false
	title := ""
	titleSet := false
	order := 0
	orderSet := false
	for _, name := range bp.GoFiles {
		file := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return websitePage{}, false, err
		}
		local, dot, ok := boleroUIName(f)
		if ok && isPageFunc(f, local, dot) {
			found = true
		}
		t, ts, o, os, err := pageConsts(f)
		if err != nil {
			return websitePage{}, false, fmt.Errorf("%s: %w", file, err)
		}
		if ts {
			if titleSet {
				return websitePage{}, false, fmt.Errorf("%s: PageTitle redeclared", file)
			}
			title, titleSet = t, true
		}
		if os {
			if orderSet {
				return websitePage{}, false, fmt.Errorf("%s: PageOrder redeclared", file)
			}
			order, orderSet = o, true
		}
	}
	if !found {
		return websitePage{}, false, nil
	}
	if !titleSet {
		title = humanizeSegment(path.Base(rel))
	}
	return websitePage{
		Import: path.Join(modPath, rel),
		URL:    "/" + rel,
		Title:  title,
		Order:  order,
	}, true, nil
}

func boleroUIName(f *ast.File) (local string, dot, ok bool) {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != "bolero/ui" {
			continue
		}
		if imp.Name == nil {
			return "ui", false, true
		}
		switch imp.Name.Name {
		case ".":
			return "", true, true
		case "_":
			return "", false, false
		default:
			return imp.Name.Name, false, true
		}
	}
	return "", false, false
}

func isPageFunc(f *ast.File, local string, dot bool) bool {
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "Page" || fn.Body == nil {
			continue
		}
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			continue
		}
		if fieldCount(fn.Type.Params) != 1 || fieldCount(fn.Type.Results) != 1 {
			continue
		}
		if !isStarNamed(fn.Type.Params.List[0].Type, local, "Context", dot) {
			continue
		}
		if !isNamed(fn.Type.Results.List[0].Type, local, "Page", dot) {
			continue
		}
		return true
	}
	return false
}

func fieldCount(list *ast.FieldList) int {
	if list == nil {
		return 0
	}
	n := 0
	for _, f := range list.List {
		if len(f.Names) == 0 {
			n++
			continue
		}
		n += len(f.Names)
	}
	return n
}

func isStarNamed(expr ast.Expr, local, typeName string, dot bool) bool {
	expr = unparen(expr)
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	return isNamed(star.X, local, typeName, dot)
}

func isNamed(expr ast.Expr, local, typeName string, dot bool) bool {
	expr = unparen(expr)
	if dot {
		id, ok := expr.(*ast.Ident)
		return ok && id.Name == typeName
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == local && sel.Sel.Name == typeName
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}

func pageConsts(f *ast.File) (title string, titleSet bool, order int, orderSet bool, err error) {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, spec := range g.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if name.Name != "PageTitle" && name.Name != "PageOrder" {
					continue
				}
				if vs.Values == nil || i >= len(vs.Values) {
					return "", false, 0, false, fmt.Errorf("%s must be a literal", name.Name)
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					return "", false, 0, false, fmt.Errorf("%s must be a literal", name.Name)
				}
				switch name.Name {
				case "PageTitle":
					if lit.Kind != token.STRING {
						return "", false, 0, false, fmt.Errorf("PageTitle must be a string")
					}
					title, err = strconv.Unquote(lit.Value)
					if err != nil {
						return "", false, 0, false, err
					}
					titleSet = true
				case "PageOrder":
					if lit.Kind != token.INT {
						return "", false, 0, false, fmt.Errorf("PageOrder must be an integer")
					}
					order, err = strconv.Atoi(lit.Value)
					if err != nil {
						return "", false, 0, false, fmt.Errorf("PageOrder must be an integer")
					}
					orderSet = true
				}
			}
		}
	}
	return title, titleSet, order, orderSet, nil
}

func humanizeSegment(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

func appDisplayName(modRoot, modPath string) string {
	_, name, ok := readAppJSON(modRoot)
	if ok && name != "" {
		return name
	}
	base := path.Base(modPath)
	if base == "." || base == "/" || base == "" {
		return "Bolero"
	}
	return base
}

func readAppJSON(dir string) (modulePath, name string, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, "app.json"))
	if err != nil {
		return "", "", false
	}
	var probe struct {
		Module string `json:"module"`
		Name   string `json:"name"`
	}
	if json.Unmarshal(data, &probe) != nil || probe.Module == "" {
		return "", "", false
	}
	return probe.Module, probe.Name, true
}

func renderWebsiteFile(pkg string, plan websitePlan) ([]byte, error) {
	alpha := slices.Clone(plan.pages)
	slices.SortFunc(alpha, func(a, b websitePage) int {
		return strings.Compare(a.Import, b.Import)
	})
	alias := make(map[string]int, len(alpha))
	for i, page := range alpha {
		alias[page.Import] = i
	}

	var b strings.Builder
	b.WriteString("// Code generated by the bolero command; DO NOT EDIT.\n\npackage ")
	b.WriteString(pkg)
	b.WriteString("\n\nimport (\n\t\"bolero/ui\"\n")
	if len(alpha) > 0 {
		b.WriteString("\n")
	}
	for i, page := range alpha {
		fmt.Fprintf(&b, "\tp%d %s\n", i, strconv.Quote(page.Import))
	}
	b.WriteString(")\n\nfunc init() {\n")
	if plan.appName != "" {
		fmt.Fprintf(&b, "\tui.SetAppName(%s)\n", strconv.Quote(plan.appName))
	}
	for _, page := range plan.pages {
		fmt.Fprintf(&b, "\tui.Register(ui.Route{Path: %s, Title: %s, Order: %d, Page: p%d.Page})\n",
			strconv.Quote(page.URL), strconv.Quote(page.Title), page.Order, alias[page.Import])
	}
	b.WriteString("}\n")
	return format.Source([]byte(b.String()))
}

func writeWebsiteFile(src []byte) (string, error) {
	sum := sha256.Sum256(src)
	dir, _, err := cache.DefaultDir()
	if err != nil || dir == "" || dir == "off" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "bolero-pages")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return "", err
	}
	// The registry is a Bolero file so it can call Page functions
	// declared in Bolero files. A Go file cannot.
	file := filepath.Join(dir, hex.EncodeToString(sum[:])+"._bolero_pages.bo")
	if _, err := os.Stat(file); err != nil {
		if err := os.WriteFile(file, src, 0o644); err != nil {
			return "", err
		}
	}
	return file, nil
}
