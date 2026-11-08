// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package envcmd implements the “bolero env” command.
package envcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/build"
	"internal/buildcfg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"cmd/go/internal/base"
	"cmd/go/internal/cache"
	"cmd/go/internal/cfg"
	"cmd/go/internal/fsys"
	"cmd/go/internal/load"
	"cmd/go/internal/modload"
	"cmd/go/internal/work"
	"cmd/internal/quoted"
	"cmd/internal/telemetry"
)

var CmdEnv = &base.Command{
	UsageLine: "bolero env [-json] [-changed] [-u] [-w] [var ...]",
	Short:     "print Bolero environment information",
	Long: `
Env prints Bolero environment information.

By default env prints information as a shell script
(on Windows, a batch file). If one or more variable
names is given as arguments, env prints the value of
each named variable on its own line.

The -json flag prints the environment in JSON format
instead of as a shell script.

The -u flag requires one or more arguments and unsets
the default setting for the named environment variables,
if one has been set with 'bolero env -w'.

The -w flag requires one or more arguments of the
form NAME=VALUE and changes the default settings
of the named environment variables to the given values.

The -changed flag prints only those settings whose effective
value differs from the default value that would be obtained in
an empty environment with no prior uses of the -w flag.

For more about environment variables, see 'bolero help environment'.
	`,
}

func init() {
	CmdEnv.Run = runEnv // break init cycle
	base.AddChdirFlag(&CmdEnv.Flag)
	base.AddBuildFlagsNX(&CmdEnv.Flag)
}

var (
	envJson    = CmdEnv.Flag.Bool("json", false, "")
	envU       = CmdEnv.Flag.Bool("u", false, "")
	envW       = CmdEnv.Flag.Bool("w", false, "")
	envChanged = CmdEnv.Flag.Bool("changed", false, "")
)

func MkEnv() []cfg.EnvVar {
	envFile, envFileChanged, _ := cfg.EnvFile()
	env := []cfg.EnvVar{
		// NOTE: Keep this list (and in general, all lists in source code) sorted by name.
		{Name: "BOLERO_MODULE", Value: cfg.Getenv("BOLERO_MODULE")},
		{Name: "GOARCH", Value: cfg.Goarch, Changed: cfg.Goarch != runtime.GOARCH},
		{Name: "BOLERO_AUTH", Value: cfg.GOAUTH, Changed: cfg.GOAUTHChanged},
		{Name: "BOLERO_BIN", Value: cfg.GOBIN},
		{Name: "BOLERO_CACHE"},
		{Name: "BOLERO_CACHEPROG", Value: cfg.GOCACHEPROG, Changed: cfg.GOCACHEPROGChanged},
		{Name: "GODEBUG", Value: os.Getenv("GODEBUG")},
		{Name: "BOLERO_ENV", Value: envFile, Changed: envFileChanged},
		{Name: "GOEXE", Value: cfg.ExeSuffix},

		// List the raw value of GOEXPERIMENT, not the cleaned one.
		// The set of default experiments may change from one release
		// to the next, so a GOEXPERIMENT setting that is redundant
		// with the current toolchain might actually be relevant with
		// a different version (for example, when bisecting a regression).
		{Name: "GOEXPERIMENT", Value: cfg.RawGOEXPERIMENT},

		{Name: "GOFIPS140", Value: cfg.GOFIPS140, Changed: cfg.GOFIPS140Changed},
		{Name: "BOLERO_FLAGS", Value: cfg.Getenv("BOLERO_FLAGS")},
		{Name: "GOHOSTARCH", Value: runtime.GOARCH},
		{Name: "GOHOSTOS", Value: runtime.GOOS},
		{Name: "BOLERO_INSECURE", Value: cfg.GOINSECURE},
		{Name: "BOLERO_MODCACHE", Value: cfg.GOMODCACHE, Changed: cfg.GOMODCACHEChanged},
		{Name: "BOLERO_NOPROXY", Value: cfg.GONOPROXY, Changed: cfg.GONOPROXYChanged},
		{Name: "BOLERO_NOSUMDB", Value: cfg.GONOSUMDB, Changed: cfg.GONOSUMDBChanged},
		{Name: "GOOS", Value: cfg.Goos, Changed: cfg.Goos != runtime.GOOS},
		{Name: "BOLERO_PATH", Value: cfg.BuildContext.GOPATH, Changed: cfg.GOPATHChanged},
		{Name: "BOLERO_PRIVATE", Value: cfg.GOPRIVATE},
		{Name: "BOLERO_PROXY", Value: cfg.GOPROXY, Changed: cfg.GOPROXYChanged},
		{Name: "BOLERO_ROOT", Value: cfg.GOROOT},
		{Name: "BOLERO_SUMDB", Value: cfg.GOSUMDB, Changed: cfg.GOSUMDBChanged},
		{Name: "BOLERO_TELEMETRY", Value: telemetry.Mode()},
		{Name: "BOLERO_TELEMETRYDIR", Value: telemetry.Dir()},
		{Name: "BOLERO_TMPDIR", Value: cfg.Getenv("BOLERO_TMPDIR")},
		{Name: "BOLERO_TOOLCHAIN"},
		{Name: "BOLERO_TOOLDIR", Value: build.ToolDir},
		{Name: "BOLERO_VCS", Value: cfg.GOVCS},
		{Name: "BOLERO_VERSION", Value: runtime.Version()},
	}

	for i := range env {
		switch env[i].Name {
		case "BOLERO_MODULE":
			if env[i].Value != "on" && env[i].Value != "" {
				env[i].Changed = true
			}
		case "BOLERO_BIN", "GOEXPERIMENT", "BOLERO_FLAGS", "BOLERO_INSECURE", "BOLERO_PRIVATE", "BOLERO_TMPDIR", "BOLERO_VCS":
			if env[i].Value != "" {
				env[i].Changed = true
			}
		case "BOLERO_CACHE":
			env[i].Value, env[i].Changed, _ = cache.DefaultDir()
		case "BOLERO_TOOLCHAIN":
			env[i].Value, env[i].Changed = cfg.EnvOrAndChanged("BOLERO_TOOLCHAIN", "")
		case "GODEBUG":
			env[i].Changed = env[i].Value != ""
		}
	}

	if work.GccgoBin != "" {
		env = append(env, cfg.EnvVar{Name: "GCCGO", Value: work.GccgoBin, Changed: true})
	} else {
		env = append(env, cfg.EnvVar{Name: "GCCGO", Value: work.GccgoName, Changed: work.GccgoChanged})
	}

	goarch, val, changed := cfg.GetArchEnv()
	if goarch != "" {
		env = append(env, cfg.EnvVar{Name: goarch, Value: val, Changed: changed})
	}

	cc := cfg.Getenv("CC")
	ccChanged := true
	if cc == "" {
		ccChanged = false
		cc = cfg.DefaultCC(cfg.Goos, cfg.Goarch)
	}
	cxx := cfg.Getenv("CXX")
	cxxChanged := true
	if cxx == "" {
		cxxChanged = false
		cxx = cfg.DefaultCXX(cfg.Goos, cfg.Goarch)
	}
	ar, arChanged := cfg.EnvOrAndChanged("AR", "ar")
	env = append(env, cfg.EnvVar{Name: "AR", Value: ar, Changed: arChanged})
	env = append(env, cfg.EnvVar{Name: "CC", Value: cc, Changed: ccChanged})
	env = append(env, cfg.EnvVar{Name: "CXX", Value: cxx, Changed: cxxChanged})

	if cfg.BuildContext.CgoEnabled {
		env = append(env, cfg.EnvVar{Name: "CGO_ENABLED", Value: "1", Changed: cfg.CGOChanged})
	} else {
		env = append(env, cfg.EnvVar{Name: "CGO_ENABLED", Value: "0", Changed: cfg.CGOChanged})
	}

	return env
}

func findEnv(env []cfg.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	if cfg.CanGetenv(name) {
		return cfg.Getenv(name)
	}
	return ""
}

// ExtraEnvVars returns environment variables that should not leak into child processes.
func ExtraEnvVars(loaderstate *modload.State) []cfg.EnvVar {
	gomod := ""
	modload.Init(loaderstate)
	if loaderstate.HasModRoot() {
		gomod = loaderstate.ModFilePath()
	} else if loaderstate.Enabled() {
		gomod = os.DevNull
	}
	loaderstate.InitWorkfile()
	gowork := modload.WorkFilePath(loaderstate)
	// As a special case, if a user set off explicitly, report that in BOLERO_WORK.
	if cfg.Getenv("BOLERO_WORK") == "off" {
		gowork = "off"
	}
	return []cfg.EnvVar{
		{Name: "BOLERO_MOD", Value: gomod},
		{Name: "BOLERO_WORK", Value: gowork},
	}
}

// ExtraEnvVarsCostly returns environment variables that should not leak into child processes
// but are costly to evaluate.
func ExtraEnvVarsCostly(loaderstate *modload.State) []cfg.EnvVar {
	b := work.NewBuilder("", loaderstate.VendorDirOrEmpty)
	defer func() {
		if err := b.Close(); err != nil {
			base.Fatal(err)
		}
	}()

	cppflags, cflags, cxxflags, fflags, ldflags, err := b.CFlags(&load.Package{})
	if err != nil {
		// Should not happen - b.CFlags was given an empty package.
		fmt.Fprintf(os.Stderr, "bolero: invalid cflags: %v\n", err)
		return nil
	}
	cmd := b.GccCmd(".", "")

	join := func(s []string) string {
		q, err := quoted.Join(s)
		if err != nil {
			return strings.Join(s, " ")
		}
		return q
	}

	ret := []cfg.EnvVar{
		// Note: Update the switch in runEnv below when adding to this list.
		{Name: "CGO_CFLAGS", Value: join(cflags)},
		{Name: "CGO_CPPFLAGS", Value: join(cppflags)},
		{Name: "CGO_CXXFLAGS", Value: join(cxxflags)},
		{Name: "CGO_FFLAGS", Value: join(fflags)},
		{Name: "CGO_LDFLAGS", Value: join(ldflags)},
		{Name: "PKG_CONFIG", Value: b.PkgconfigCmd()},
		{Name: "GOGCCFLAGS", Value: join(cmd[3:])},
	}

	for i := range ret {
		ev := &ret[i]
		switch ev.Name {
		case "GOGCCFLAGS": // GOGCCFLAGS cannot be modified
		case "CGO_CPPFLAGS":
			ev.Changed = ev.Value != ""
		case "PKG_CONFIG":
			ev.Changed = ev.Value != cfg.DefaultPkgConfig
		case "CGO_CXXFLAGS", "CGO_CFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS":
			ev.Changed = ev.Value != work.DefaultCFlags
		}
	}

	return ret
}

// argKey returns the KEY part of the arg KEY=VAL, or else arg itself.
func argKey(arg string) string {
	i := strings.Index(arg, "=")
	if i < 0 {
		return arg
	}
	return arg[:i]
}

func runEnv(ctx context.Context, cmd *base.Command, args []string) {
	moduleLoaderState := modload.NewState()
	if *envJson && *envU {
		base.Fatalf("bolero: cannot use -json with -u")
	}
	if *envJson && *envW {
		base.Fatalf("bolero: cannot use -json with -w")
	}
	if *envU && *envW {
		base.Fatalf("bolero: cannot use -u with -w")
	}

	// Handle 'bolero env -w' and 'bolero env -u' before calling buildcfg.Check,
	// so they can be used to recover from an invalid configuration.
	if *envW {
		runEnvW(args)
		return
	}

	if *envU {
		runEnvU(args)
		return
	}

	buildcfg.Check()
	if cfg.ExperimentErr != nil {
		base.Fatal(cfg.ExperimentErr)
	}

	for _, arg := range args {
		if strings.Contains(arg, "=") {
			base.Fatalf("bolero: invalid variable name %q (use -w to set variable)", arg)
		}
	}

	env := cfg.CmdEnv
	env = append(env, ExtraEnvVars(moduleLoaderState)...)

	if err := fsys.Init(); err != nil {
		base.Fatal(err)
	}

	// Do we need to call ExtraEnvVarsCostly, which is a bit expensive?
	needCostly := false
	if len(args) == 0 {
		// We're listing all environment variables ("bolero env"),
		// including the expensive ones.
		needCostly = true
	} else {
		needCostly = false
	checkCostly:
		for _, arg := range args {
			switch argKey(arg) {
			case "CGO_CFLAGS",
				"CGO_CPPFLAGS",
				"CGO_CXXFLAGS",
				"CGO_FFLAGS",
				"CGO_LDFLAGS",
				"PKG_CONFIG",
				"GOGCCFLAGS":
				needCostly = true
				break checkCostly
			}
		}
	}
	if needCostly {
		work.BuildInit(moduleLoaderState)
		env = append(env, ExtraEnvVarsCostly(moduleLoaderState)...)
	}

	if len(args) > 0 {
		// Show only the named vars.
		if !*envChanged {
			if *envJson {
				es := make([]cfg.EnvVar, 0, len(args))
				for _, name := range args {
					e := cfg.EnvVar{Name: name, Value: findEnv(env, name)}
					es = append(es, e)
				}
				env = es
			} else {
				// Print just the values, without names.
				for _, name := range args {
					fmt.Printf("%s\n", findEnv(env, name))
				}
				return
			}
		} else {
			// Show only the changed, named vars.
			var es []cfg.EnvVar
			for _, name := range args {
				for _, e := range env {
					if e.Name == name {
						es = append(es, e)
						break
					}
				}
			}
			env = es
		}
	}

	// print
	if *envJson {
		printEnvAsJSON(env, *envChanged)
	} else {
		PrintEnv(os.Stdout, env, *envChanged)
	}
}

func runEnvW(args []string) {
	// Process and sanity-check command line.
	if len(args) == 0 {
		base.Fatalf("bolero: no KEY=VALUE arguments given")
	}
	osEnv := make(map[string]string)
	for _, e := range cfg.OrigEnv {
		if i := strings.Index(e, "="); i >= 0 {
			osEnv[e[:i]] = e[i+1:]
		}
	}
	add := make(map[string]string)
	for _, arg := range args {
		key, val, found := strings.Cut(arg, "=")
		if !found {
			base.Fatalf("bolero: arguments must be KEY=VALUE: invalid argument: %s", arg)
		}
		if err := checkEnvWrite(key, val); err != nil {
			base.Fatal(err)
		}
		if _, ok := add[key]; ok {
			base.Fatalf("bolero: multiple values for key: %s", key)
		}
		add[key] = val
		if osVal := osEnv[key]; osVal != "" && osVal != val {
			fmt.Fprintf(os.Stderr, "warning: bolero env -w %s=... does not override conflicting OS environment variable\n", key)
		}
	}

	if err := checkBuildConfig(add, nil); err != nil {
		base.Fatal(err)
	}

	gotmp, okGOTMP := add["BOLERO_TMPDIR"]
	if okGOTMP {
		if !filepath.IsAbs(gotmp) && gotmp != "" {
			base.Fatalf("bolero: BOLERO_TMPDIR must be an absolute path")
		}
	}

	updateEnvFile(add, nil)
}

func runEnvU(args []string) {
	// Process and sanity-check command line.
	if len(args) == 0 {
		base.Fatalf("bolero: 'bolero env -u' requires an argument")
	}
	del := make(map[string]bool)
	for _, arg := range args {
		if err := checkEnvWrite(arg, ""); err != nil {
			base.Fatal(err)
		}
		del[arg] = true
	}

	if err := checkBuildConfig(nil, del); err != nil {
		base.Fatal(err)
	}

	updateEnvFile(nil, del)
}

// checkBuildConfig checks whether the build configuration is valid
// after the specified configuration environment changes are applied.
func checkBuildConfig(add map[string]string, del map[string]bool) error {
	// get returns the value for key after applying add and del and
	// reports whether it changed. cur should be the current value
	// (i.e., before applying changes) and def should be the default
	// value (i.e., when no environment variables are provided at all).
	get := func(key, cur, def string) (string, bool) {
		if val, ok := add[key]; ok {
			return val, true
		}
		if del[key] {
			val := getOrigEnv(key)
			if val == "" {
				val = def
			}
			return val, true
		}
		return cur, false
	}

	goos, okGOOS := get("GOOS", cfg.Goos, build.Default.GOOS)
	goarch, okGOARCH := get("GOARCH", cfg.Goarch, build.Default.GOARCH)
	if okGOOS || okGOARCH {
		if err := work.CheckGOOSARCHPair(goos, goarch); err != nil {
			return err
		}
	}

	goexperiment, okGOEXPERIMENT := get("GOEXPERIMENT", cfg.RawGOEXPERIMENT, buildcfg.DefaultGOEXPERIMENT)
	if okGOEXPERIMENT {
		if _, err := buildcfg.ParseGOEXPERIMENT(goos, goarch, goexperiment); err != nil {
			return err
		}
	}

	return nil
}

// PrintEnv prints the environment variables to w.
func PrintEnv(w io.Writer, env []cfg.EnvVar, onlyChanged bool) {
	env = slices.Clone(env)
	slices.SortFunc(env, func(x, y cfg.EnvVar) int { return strings.Compare(x.Name, y.Name) })

	for _, e := range env {
		if e.Name != "TERM" {
			if runtime.GOOS != "plan9" && bytes.Contains([]byte(e.Value), []byte{0}) {
				base.Fatalf("bolero: internal error: encountered null byte in environment variable %s on non-plan9 platform", e.Name)
			}
			if onlyChanged && !e.Changed {
				continue
			}
			switch runtime.GOOS {
			default:
				fmt.Fprintf(w, "%s=%s\n", e.Name, shellQuote(e.Value))
			case "plan9":
				if strings.IndexByte(e.Value, '\x00') < 0 {
					fmt.Fprintf(w, "%s='%s'\n", e.Name, strings.ReplaceAll(e.Value, "'", "''"))
				} else {
					v := strings.Split(e.Value, "\x00")
					fmt.Fprintf(w, "%s=(", e.Name)
					for x, s := range v {
						if x > 0 {
							fmt.Fprintf(w, " ")
						}
						fmt.Fprintf(w, "'%s'", strings.ReplaceAll(s, "'", "''"))
					}
					fmt.Fprintf(w, ")\n")
				}
			case "windows":
				if hasNonGraphic(e.Value) {
					base.Errorf("bolero: stripping unprintable or unescapable characters from %%%q%%", e.Name)
				}
				fmt.Fprintf(w, "set %s=%s\n", e.Name, batchEscape(e.Value))
			}
		}
	}
}

// isWindowsUnquotableRune reports whether r can't be quoted in a
// Windows "set" command.
// These runes will be replaced by the Unicode replacement character.
func isWindowsUnquotableRune(r rune) bool {
	if r == '\r' || r == '\n' {
		return true
	}
	return !unicode.IsGraphic(r) && !unicode.IsSpace(r)
}

func hasNonGraphic(s string) bool {
	return strings.ContainsFunc(s, isWindowsUnquotableRune)
}

func shellQuote(s string) string {
	var sb strings.Builder
	sb.WriteByte('\'')
	for _, r := range s {
		if r == '\'' {
			// Close the single quoted string, add an escaped single quote,
			// and start another single quoted string.
			sb.WriteString(`'\''`)
		} else {
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('\'')
	return sb.String()
}

func batchEscape(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if isWindowsUnquotableRune(r) {
			sb.WriteRune(unicode.ReplacementChar)
			continue
		}
		switch r {
		case '%':
			sb.WriteString("%%")
		case '<', '>', '|', '&', '^':
			// These are special characters that need to be escaped with ^. See
			// https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/set_1.
			sb.WriteByte('^')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func printEnvAsJSON(env []cfg.EnvVar, onlyChanged bool) {
	m := make(map[string]string)
	for _, e := range env {
		if e.Name == "TERM" {
			continue
		}
		if onlyChanged && !e.Changed {
			continue
		}
		m[e.Name] = e.Value
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "\t")
	if err := enc.Encode(m); err != nil {
		base.Fatalf("bolero: %s", err)
	}
}

func getOrigEnv(key string) string {
	for _, v := range cfg.OrigEnv {
		if v, found := strings.CutPrefix(v, key+"="); found {
			return v
		}
	}
	return ""
}

func checkEnvWrite(key, val string) error {
	switch key {
	case "GOEXE", "GOGCCFLAGS", "GOHOSTARCH", "GOHOSTOS", "BOLERO_MOD", "BOLERO_WORK", "BOLERO_TOOLDIR", "BOLERO_VERSION", "BOLERO_TELEMETRY", "BOLERO_TELEMETRYDIR":
		return fmt.Errorf("%s cannot be modified", key)
	case "BOLERO_ENV", "GODEBUG":
		return fmt.Errorf("%s can only be set using the OS environment", key)
	}

	// To catch typos and the like, check that we know the variable.
	// If it's already in the env file, we assume it's known.
	if !cfg.CanGetenv(key) {
		return fmt.Errorf("unknown bolero command variable %s", key)
	}

	// Some variables can only have one of a few valid values. If set to an
	// invalid value, the next cmd/go invocation might fail immediately,
	// even 'bolero env -w' itself.
	switch key {
	case "BOLERO_MODULE":
		switch val {
		case "", "auto", "on", "off":
		default:
			return fmt.Errorf("invalid %s value %q", key, val)
		}
	case "BOLERO_PATH":
		if strings.HasPrefix(val, "~") {
			return fmt.Errorf("BOLERO_PATH entry cannot start with shell metacharacter '~': %q", val)
		}
		if !filepath.IsAbs(val) && val != "" {
			return fmt.Errorf("BOLERO_PATH entry is relative; must be absolute path: %q", val)
		}
	case "BOLERO_MODCACHE":
		if !filepath.IsAbs(val) && val != "" {
			return fmt.Errorf("BOLERO_MODCACHE entry is relative; must be absolute path: %q", val)
		}
	case "CC", "CXX":
		if val == "" {
			break
		}
		args, err := quoted.Split(val)
		if err != nil {
			return fmt.Errorf("invalid %s: %v", key, err)
		}
		if len(args) == 0 {
			return fmt.Errorf("%s entry cannot contain only space", key)
		}
		if !filepath.IsAbs(args[0]) && args[0] != filepath.Base(args[0]) {
			return fmt.Errorf("%s entry is relative; must be absolute path: %q", key, args[0])
		}
	}

	if !utf8.ValidString(val) {
		return fmt.Errorf("invalid UTF-8 in %s=... value", key)
	}
	if strings.Contains(val, "\x00") {
		return fmt.Errorf("invalid NUL in %s=... value", key)
	}
	if strings.ContainsAny(val, "\v\r\n") {
		return fmt.Errorf("invalid newline in %s=... value", key)
	}
	return nil
}

func readEnvFileLines(mustExist bool) []string {
	file, _, err := cfg.EnvFile()
	if file == "" {
		if mustExist {
			base.Fatalf("bolero: cannot find bolero env config: %v", err)
		}
		return nil
	}
	data, err := os.ReadFile(file)
	if err != nil && (!os.IsNotExist(err) || mustExist) {
		base.Fatalf("bolero: reading bolero env config: %v", err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	} else {
		lines[len(lines)-1] += "\n"
	}
	return lines
}

func updateEnvFile(add map[string]string, del map[string]bool) {
	lines := readEnvFileLines(len(add) == 0)

	// Delete all but last copy of any duplicated variables,
	// since the last copy is the one that takes effect.
	prev := make(map[string]int)
	for l, line := range lines {
		if key := lineToKey(line); key != "" {
			if p, ok := prev[key]; ok {
				lines[p] = ""
			}
			prev[key] = l
		}
	}

	// Add variables (bolero env -w). Update existing lines in file if present, add to end otherwise.
	for key, val := range add {
		if p, ok := prev[key]; ok {
			lines[p] = key + "=" + val + "\n"
			delete(add, key)
		}
	}
	for key, val := range add {
		lines = append(lines, key+"="+val+"\n")
	}

	// Delete requested variables (bolero env -u).
	for key := range del {
		if p, ok := prev[key]; ok {
			lines[p] = ""
		}
	}

	// Sort runs of KEY=VALUE lines
	// (that is, blocks of lines where blocks are separated
	// by comments, blank lines, or invalid lines).
	start := 0
	for i := 0; i <= len(lines); i++ {
		if i == len(lines) || lineToKey(lines[i]) == "" {
			sortKeyValues(lines[start:i])
			start = i + 1
		}
	}

	file, _, err := cfg.EnvFile()
	if file == "" {
		base.Fatalf("bolero: cannot find bolero env config: %v", err)
	}
	data := []byte(strings.Join(lines, ""))
	err = os.WriteFile(file, data, 0666)
	if err != nil {
		// Try creating directory.
		os.MkdirAll(filepath.Dir(file), 0777)
		err = os.WriteFile(file, data, 0666)
		if err != nil {
			base.Fatalf("bolero: writing bolero env config: %v", err)
		}
	}
}

// lineToKey returns the KEY part of the line KEY=VALUE or else an empty string.
func lineToKey(line string) string {
	i := strings.Index(line, "=")
	if i < 0 || strings.Contains(line[:i], "#") {
		return ""
	}
	return line[:i]
}

// sortKeyValues sorts a sequence of lines by key.
// It differs from sort.Strings in that GO386= sorts after GO=.
func sortKeyValues(lines []string) {
	sort.Slice(lines, func(i, j int) bool {
		return lineToKey(lines[i]) < lineToKey(lines[j])
	})
}
