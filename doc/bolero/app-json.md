# app.json

`app.json` is the manifest of a Bolero project. It replaces `go.mod` for the main module. Downloaded dependencies, the standard library, and the toolchain itself keep their `go.mod` files. `go.sum` keeps its name, format, and place next to the manifest.

The module root is the nearest parent directory that contains `app.json` or `go.mod`. A directory that contains both is an error.

`go.work` workspaces are unchanged and list modules that have a `go.mod`. A workspace module is created with `bolero mod init -modfile=go.mod`.

There is no toolchain field. Bolero stays on the local toolchain. The `go` field is the language version only, with the same grammar as the `go` line in a `go.mod` file.

## File

UTF-8 JSON, two-space indent, one trailing newline. The `go` command rewrites the file in the field order below and omits empty fields. Duplicate keys are errors. An unrecognized field is an error.

```json
{
  "module": "example.com/shop",
  "go": "1.26",
  "name": "Shop",
  "version": "0.1.0",
  "description": "Wholesale order desk",
  "license": "MIT",
  "authors": [
    { "name": "Ada Lovelace", "email": "ada@example.com" }
  ],
  "website": "https://example.com",
  "require": {
    "example.com/widgets": "v1.4.2"
  },
  "indirect": {
    "golang.org/x/text": "v0.21.0"
  },
  "exclude": {
    "example.com/widgets": ["v1.3.0"]
  },
  "replace": [
    { "module": "example.com/widgets", "with": "../widgets" }
  ],
  "retract": [
    { "version": "v0.9.0", "rationale": "published accidentally" }
  ],
  "tool": ["example.com/lint"],
  "ignore": ["./third_party"],
  "godebug": { "default": "go1.26" }
}
```

`module` and `go` are required. The other identity fields are optional: `name`, `version` (`major.minor.patch`, no `v` prefix), `description`, `license` (an SPDX identifier, stored unchecked), `authors` (each `name` required, `email` optional), and `website`. The build does not read them. `version` here is the application version, not a module version.

The module-graph fields have the same meaning as the matching `go.mod` directives. Versions are exact, the same strings a `go.mod` file uses. A path listed in both `require` and `indirect` is an error. A `replace` `with` of `./…`, `../…`, or an absolute path is a directory relative to the `app.json` file. A retract is either `version` or both `low` and `high`. `rationale` is the text that a `go.mod` file stores as a comment.

## Commands

`bolero mod init [module-path]` writes `app.json` in the current directory:

```json
{
  "module": "example.com/shop",
  "go": "1.26.3",
  "version": "0.1.0",
  "license": "MIT",
  "authors": [
    { "name": "Your Name", "email": "you@example.com" }
  ]
}
```

The `go` value is the version of the toolchain running the command. Init does not write `go.sum`. If `app.json` or `go.mod` already exists, init stops.

`bolero get`, `bolero mod tidy`, and `bolero mod edit` read and write `app.json` when it is the main module file. They keep the identity fields. `go.sum` is updated by the same rules as for a `go.mod` module.

`-modfile` names an alternate manifest. A file named `app.json` uses this format. Any other name uses `go.mod` syntax, and the checksum file is that name with `.mod` replaced by `.sum`.
