// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"cmd/go/internal/base"
	"cmd/go/internal/fsys"
	"internal/bolero"
	"internal/lazyregexp"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	defaultAppVersion = "0.1.0"
	defaultAppLicense = "MIT"
	defaultAuthorName = "Your Name"
	defaultAuthorMail = "you@example.com"
)

// appVersionRE matches an application version, major.minor.patch,
// without a v prefix and without a pre-release suffix.
var appVersionRE = lazyregexp.New(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// IsAppJSON reports whether path names a Bolero project manifest.
func IsAppJSON(path string) bool {
	return filepath.Base(path) == bolero.AppJSON
}

// moduleFile returns the manifest in modRoot.
// A directory with neither file reports the go.mod path.
func moduleFile(modRoot string) string {
	app := filepath.Join(modRoot, bolero.AppJSON)
	if fi, err := fsys.Stat(app); err == nil && !fi.IsDir() {
		return app
	}
	return filepath.Join(modRoot, "go.mod")
}

// goSumFor returns the go.sum path that belongs next to the manifest.
func goSumFor(modFile string) string {
	if IsAppJSON(modFile) {
		return filepath.Join(filepath.Dir(modFile), "go.sum")
	}
	return strings.TrimSuffix(modFile, ".mod") + ".sum"
}

func formatManifest(path string, f *modfile.File) ([]byte, error) {
	if IsAppJSON(path) {
		return formatAppJSON(path, f)
	}
	return f.Format()
}

// ParseAppJSON parses an app.json manifest into a module file.
// Identity fields are remembered for a later FormatAppJSON of the same path.
func ParseAppJSON(filename string, data []byte) (*modfile.File, error) {
	return parseAppJSON(filename, data, nil)
}

// FormatAppJSON writes f as an app.json manifest.
// Identity fields remembered for path are kept.
// A toolchain line is not written.
func FormatAppJSON(path string, f *modfile.File) ([]byte, error) {
	return formatAppJSON(path, f)
}

// AppGoVersion returns the language version declared in an app.json file.
func AppGoVersion(data []byte) (string, error) {
	f, err := parseAppJSON(bolero.AppJSON, data, nil)
	if err != nil {
		return "", err
	}
	return f.Go.Version, nil
}

type appFile struct {
	Module      string              `json:"module"`
	Go          string              `json:"go"`
	Name        string              `json:"name,omitempty"`
	Version     string              `json:"version,omitempty"`
	Description string              `json:"description,omitempty"`
	License     string              `json:"license,omitempty"`
	Authors     []appAuthor         `json:"authors,omitempty"`
	Website     string              `json:"website,omitempty"`
	Require     map[string]string   `json:"require,omitempty"`
	Indirect    map[string]string   `json:"indirect,omitempty"`
	Exclude     map[string][]string `json:"exclude,omitempty"`
	Replace     []appReplace        `json:"replace,omitempty"`
	Retract     []appRetract        `json:"retract,omitempty"`
	Tool        []string            `json:"tool,omitempty"`
	Ignore      []string            `json:"ignore,omitempty"`
	Godebug     map[string]string   `json:"godebug,omitempty"`
}

type appAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type appReplace struct {
	Module      string `json:"module"`
	Version     string `json:"version,omitempty"`
	With        string `json:"with"`
	WithVersion string `json:"withVersion,omitempty"`
}

type appRetract struct {
	Version   string `json:"version,omitempty"`
	Low       string `json:"low,omitempty"`
	High      string `json:"high,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

type appIdentity struct {
	Name        string
	Version     string
	Description string
	License     string
	Website     string
	Authors     []appAuthor
}

var appIdentities sync.Map // cleaned path → appIdentity

func defaultAppIdentity() appIdentity {
	return appIdentity{
		Version: defaultAppVersion,
		License: defaultAppLicense,
		Authors: []appAuthor{{Name: defaultAuthorName, Email: defaultAuthorMail}},
	}
}

func rememberAppIdentity(path string, id appIdentity) {
	id.Authors = append([]appAuthor(nil), id.Authors...)
	appIdentities.Store(filepath.Clean(path), id)
}

func rememberedAppIdentity(path string) appIdentity {
	v, ok := appIdentities.Load(filepath.Clean(path))
	if !ok {
		return appIdentity{}
	}
	id := v.(appIdentity)
	id.Authors = append([]appAuthor(nil), id.Authors...)
	return id
}

func (a appFile) identity() appIdentity {
	return appIdentity{
		Name:        a.Name,
		Version:     a.Version,
		Description: a.Description,
		License:     a.License,
		Website:     a.Website,
		Authors:     append([]appAuthor(nil), a.Authors...),
	}
}

func parseAppJSON(filename string, data []byte, fix modfile.VersionFixer) (*modfile.File, error) {
	short := base.ShortPath(filename)
	if err := rejectDuplicateKeys(data); err != nil {
		if strings.HasPrefix(err.Error(), "duplicate field") {
			return nil, fmt.Errorf("%s: %w", short, err)
		}
		return nil, fmt.Errorf("%s: invalid JSON: %v", short, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var app appFile
	if err := dec.Decode(&app); err != nil {
		return nil, fmt.Errorf("%s: %s", short, jsonFieldError(err))
	}
	if dec.More() {
		return nil, fmt.Errorf("%s: invalid JSON: extra data after value", short)
	}
	if err := app.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", short, err)
	}
	f, err := modfile.Parse(short, app.moduleText(), fix)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", short, err)
	}
	rememberAppIdentity(filename, app.identity())
	return f, nil
}

func jsonFieldError(err error) string {
	msg := strings.TrimPrefix(err.Error(), "json: ")
	return msg
}

func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return walkJSON(dec)
}

func walkJSON(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("invalid object key")
			}
			if seen[key] {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = true
			if err := walkJSON(dec); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := walkJSON(dec); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	default:
		return fmt.Errorf("invalid JSON")
	}
}

func (a *appFile) validate() error {
	if a.Module == "" {
		return fmt.Errorf("module: module path is required")
	}
	if err := module.CheckPath(a.Module); err != nil {
		return fmt.Errorf("module: invalid module path %q: %v", a.Module, err)
	}
	if err := CheckReservedModulePath(a.Module); err != nil {
		return fmt.Errorf("module: invalid module path %q: %v", a.Module, err)
	}
	if a.Go == "" {
		return fmt.Errorf("bolero: language version is required")
	}
	if !modfile.GoVersionRE.MatchString(a.Go) {
		return fmt.Errorf("bolero: invalid language version %q", a.Go)
	}
	if a.Version != "" && !appVersionRE.MatchString(a.Version) {
		return fmt.Errorf("version: must be major.minor.patch")
	}
	for _, field := range []struct{ name, val string }{
		{"name", a.Name},
		{"description", a.Description},
		{"license", a.License},
		{"website", a.Website},
	} {
		if strings.ContainsAny(field.val, "\r\n") {
			return fmt.Errorf("%s: must be one line", field.name)
		}
	}
	for _, author := range a.Authors {
		if author.Name == "" {
			return fmt.Errorf("authors: name is required")
		}
		if strings.ContainsAny(author.Name, "\r\n") || strings.ContainsAny(author.Email, "\r\n") {
			return fmt.Errorf("authors: name and email must be one line")
		}
	}
	for path := range a.Require {
		if _, ok := a.Indirect[path]; ok {
			return fmt.Errorf("require: %s appears in require and indirect", path)
		}
	}
	if err := checkVersions("require", a.Require); err != nil {
		return err
	}
	if err := checkVersions("indirect", a.Indirect); err != nil {
		return err
	}
	for path, versions := range a.Exclude {
		seen := make(map[string]bool, len(versions))
		for _, vers := range versions {
			if seen[vers] {
				return fmt.Errorf("exclude: %s %s is repeated", path, vers)
			}
			seen[vers] = true
			if err := module.Check(path, vers); err != nil {
				return fmt.Errorf("exclude: %v", err)
			}
		}
	}
	replaced := make(map[module.Version]bool, len(a.Replace))
	for _, r := range a.Replace {
		if r.Module == "" || r.With == "" {
			return fmt.Errorf("replace: module and with are required")
		}
		old := module.Version{Path: r.Module, Version: r.Version}
		if replaced[old] {
			return fmt.Errorf("replace: %s is repeated", replaceOld(r))
		}
		replaced[old] = true
		if r.Version == "" {
			if err := module.CheckPath(r.Module); err != nil {
				return fmt.Errorf("replace: %v", err)
			}
		} else if err := module.Check(r.Module, r.Version); err != nil {
			return fmt.Errorf("replace: %v", err)
		}
		if modfile.IsDirectoryPath(r.With) {
			if r.WithVersion != "" {
				return fmt.Errorf("replace: directory %s has a version", r.With)
			}
			continue
		}
		if r.WithVersion == "" {
			return fmt.Errorf("replace: %s needs a version", r.With)
		}
		if err := module.Check(r.With, r.WithVersion); err != nil {
			return fmt.Errorf("replace: %v", err)
		}
	}
	_, pathMajor, _ := module.SplitPathVersion(a.Module)
	seenRetract := make(map[modfile.VersionInterval]bool, len(a.Retract))
	for _, r := range a.Retract {
		single := r.Version != ""
		interval := r.Low != "" || r.High != ""
		if single == interval {
			return fmt.Errorf("retract: need a version or a low and high pair")
		}
		var vi modfile.VersionInterval
		if single {
			if err := checkRetractVersion(a.Module, pathMajor, r.Version); err != nil {
				return err
			}
			vi = modfile.VersionInterval{Low: r.Version, High: r.Version}
		} else {
			if r.Low == "" || r.High == "" {
				return fmt.Errorf("retract: need both low and high")
			}
			if err := checkRetractVersion(a.Module, pathMajor, r.Low); err != nil {
				return err
			}
			if err := checkRetractVersion(a.Module, pathMajor, r.High); err != nil {
				return err
			}
			if semver.Compare(r.Low, r.High) > 0 {
				return fmt.Errorf("retract: low %s is after high %s", r.Low, r.High)
			}
			vi = modfile.VersionInterval{Low: r.Low, High: r.High}
		}
		if seenRetract[vi] {
			return fmt.Errorf("retract: %s is repeated", vi.Low)
		}
		seenRetract[vi] = true
		if strings.ContainsAny(r.Rationale, "\r") {
			return fmt.Errorf("retract: rationale must not contain a carriage return")
		}
	}
	if err := checkPathList("tool", a.Tool); err != nil {
		return err
	}
	seenIgnore := make(map[string]bool, len(a.Ignore))
	for _, p := range a.Ignore {
		if p == "" {
			return fmt.Errorf("ignore: path is required")
		}
		if seenIgnore[p] {
			return fmt.Errorf("ignore: %s is repeated", p)
		}
		seenIgnore[p] = true
	}
	for k, v := range a.Godebug {
		if err := CheckGodebug("godebug", k, v); err != nil {
			return fmt.Errorf("godebug: %s: %v", k, err)
		}
	}
	return nil
}

func checkVersions(field string, mods map[string]string) error {
	for path, vers := range mods {
		if err := module.Check(path, vers); err != nil {
			return fmt.Errorf("%s: %v", field, err)
		}
	}
	return nil
}

func checkRetractVersion(modPath, pathMajor, vers string) error {
	if vers != module.CanonicalVersion(vers) {
		return fmt.Errorf("retract: version %q is not canonical", vers)
	}
	if err := module.CheckPathMajor(vers, pathMajor); err != nil {
		return fmt.Errorf("retract: %s %s: %v", modPath, vers, err)
	}
	return nil
}

func checkPathList(field string, paths []string) error {
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if err := module.CheckImportPath(p); err != nil {
			return fmt.Errorf("%s: %v", field, err)
		}
		if seen[p] {
			return fmt.Errorf("%s: %s is repeated", field, p)
		}
		seen[p] = true
	}
	return nil
}

func replaceOld(r appReplace) string {
	if r.Version == "" {
		return r.Module
	}
	return r.Module + "@" + r.Version
}

func (a *appFile) moduleText() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo %s\n", modfile.AutoQuote(a.Module), a.Go)
	writeGodebug(&b, a.Godebug)
	writeRequires(&b, a.Require, a.Indirect)
	writeExcludes(&b, a.Exclude)
	writeReplaces(&b, a.Replace)
	writeRetracts(&b, a.Retract)
	writeTools(&b, a.Tool)
	writeIgnores(&b, a.Ignore)
	return []byte(b.String())
}

func writeGodebug(b *strings.Builder, godebug map[string]string) {
	if len(godebug) == 0 {
		return
	}
	b.WriteString("\n")
	for _, k := range sortedKeys(godebug) {
		fmt.Fprintf(b, "godebug %s=%s\n", k, godebug[k])
	}
}

func writeRequires(b *strings.Builder, direct, indirect map[string]string) {
	if len(direct) == 0 && len(indirect) == 0 {
		return
	}
	b.WriteString("\n")
	for _, path := range sortedKeys(direct) {
		fmt.Fprintf(b, "require %s %s\n", modfile.AutoQuote(path), direct[path])
	}
	for _, path := range sortedKeys(indirect) {
		fmt.Fprintf(b, "require %s %s // indirect\n", modfile.AutoQuote(path), indirect[path])
	}
}

func writeExcludes(b *strings.Builder, exclude map[string][]string) {
	if len(exclude) == 0 {
		return
	}
	b.WriteString("\n")
	for _, path := range sortedKeys(exclude) {
		versions := append([]string(nil), exclude[path]...)
		slices.SortFunc(versions, semver.Compare)
		for _, vers := range versions {
			fmt.Fprintf(b, "exclude %s %s\n", modfile.AutoQuote(path), vers)
		}
	}
}

func writeReplaces(b *strings.Builder, replace []appReplace) {
	if len(replace) == 0 {
		return
	}
	b.WriteString("\n")
	for _, r := range replace {
		if r.Version == "" {
			fmt.Fprintf(b, "replace %s => %s", modfile.AutoQuote(r.Module), modfile.AutoQuote(r.With))
		} else {
			fmt.Fprintf(b, "replace %s %s => %s", modfile.AutoQuote(r.Module), r.Version, modfile.AutoQuote(r.With))
		}
		if r.WithVersion != "" {
			fmt.Fprintf(b, " %s", r.WithVersion)
		}
		b.WriteByte('\n')
	}
}

func writeRetracts(b *strings.Builder, retract []appRetract) {
	if len(retract) == 0 {
		return
	}
	b.WriteString("\n")
	for _, r := range retract {
		if r.Rationale != "" {
			for line := range strings.SplitSeq(r.Rationale, "\n") {
				fmt.Fprintf(b, "// %s\n", line)
			}
		}
		if r.Version != "" {
			fmt.Fprintf(b, "retract %s\n", r.Version)
			continue
		}
		fmt.Fprintf(b, "retract [%s, %s]\n", r.Low, r.High)
	}
}

func writeTools(b *strings.Builder, tools []string) {
	if len(tools) == 0 {
		return
	}
	b.WriteString("\n")
	for _, path := range tools {
		fmt.Fprintf(b, "tool %s\n", modfile.AutoQuote(path))
	}
}

func writeIgnores(b *strings.Builder, ignore []string) {
	if len(ignore) == 0 {
		return
	}
	b.WriteString("\n")
	for _, path := range ignore {
		fmt.Fprintf(b, "ignore %s\n", modfile.AutoQuote(path))
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func formatAppJSON(path string, f *modfile.File) ([]byte, error) {
	if f == nil || f.Module == nil || f.Module.Mod.Path == "" {
		return nil, fmt.Errorf("%s: missing module path", base.ShortPath(path))
	}
	if f.Go == nil || f.Go.Version == "" {
		return nil, fmt.Errorf("%s: missing go version", base.ShortPath(path))
	}
	f.Cleanup()
	f.DropToolchainStmt()
	text, err := f.Format()
	if err != nil {
		return nil, err
	}
	parsed, err := modfile.Parse(bolero.AppJSON, text, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", base.ShortPath(path), err)
	}
	app := appFromModFile(path, parsed)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(app); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func appFromModFile(path string, f *modfile.File) appFile {
	id := rememberedAppIdentity(path)
	app := appFile{
		Module:      f.Module.Mod.Path,
		Go:          f.Go.Version,
		Name:        id.Name,
		Version:     id.Version,
		Description: id.Description,
		License:     id.License,
		Website:     id.Website,
		Authors:     id.Authors,
	}
	for _, r := range f.Require {
		if r.Mod.Path == "" {
			continue
		}
		if r.Indirect {
			if app.Indirect == nil {
				app.Indirect = make(map[string]string)
			}
			app.Indirect[r.Mod.Path] = r.Mod.Version
			continue
		}
		if app.Require == nil {
			app.Require = make(map[string]string)
		}
		app.Require[r.Mod.Path] = r.Mod.Version
	}
	for _, x := range f.Exclude {
		if x.Mod.Path == "" {
			continue
		}
		if app.Exclude == nil {
			app.Exclude = make(map[string][]string)
		}
		app.Exclude[x.Mod.Path] = append(app.Exclude[x.Mod.Path], x.Mod.Version)
	}
	for path, versions := range app.Exclude {
		slices.SortFunc(versions, semver.Compare)
		app.Exclude[path] = versions
	}
	for _, r := range f.Replace {
		if r.Old.Path == "" {
			continue
		}
		app.Replace = append(app.Replace, appReplace{
			Module:      r.Old.Path,
			Version:     r.Old.Version,
			With:        r.New.Path,
			WithVersion: r.New.Version,
		})
	}
	slices.SortFunc(app.Replace, func(a, b appReplace) int {
		if c := strings.Compare(a.Module, b.Module); c != 0 {
			return c
		}
		return semver.Compare(a.Version, b.Version)
	})
	for _, r := range f.Retract {
		if r.Low == "" && r.High == "" {
			continue
		}
		item := appRetract{Rationale: r.Rationale}
		if r.Low == r.High {
			item.Version = r.Low
		} else {
			item.Low = r.Low
			item.High = r.High
		}
		app.Retract = append(app.Retract, item)
	}
	slices.SortFunc(app.Retract, func(a, b appRetract) int {
		al, ah := retractEnds(a)
		bl, bh := retractEnds(b)
		if c := semver.Compare(al, bl); c != 0 {
			return c
		}
		return semver.Compare(ah, bh)
	})
	for _, t := range f.Tool {
		if t.Path != "" {
			app.Tool = append(app.Tool, t.Path)
		}
	}
	slices.Sort(app.Tool)
	for _, ign := range f.Ignore {
		if ign.Path != "" {
			app.Ignore = append(app.Ignore, ign.Path)
		}
	}
	slices.Sort(app.Ignore)
	for _, g := range f.Godebug {
		if g.Key == "" {
			continue
		}
		if app.Godebug == nil {
			app.Godebug = make(map[string]string)
		}
		app.Godebug[g.Key] = g.Value
	}
	return app
}

func retractEnds(r appRetract) (low, high string) {
	if r.Version != "" {
		return r.Version, r.Version
	}
	return r.Low, r.High
}
