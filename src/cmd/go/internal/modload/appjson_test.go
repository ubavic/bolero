// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modload

import (
	"strings"
	"testing"
)

func TestAppJSONRoundTrip(t *testing.T) {
	const src = `{
  "module": "example.com/shop",
  "go": "1.26",
  "name": "Shop",
  "version": "0.1.0",
  "description": "Wholesale order desk",
  "license": "MIT",
  "authors": [
    {
      "name": "Ada Lovelace",
      "email": "ada@example.com"
    },
    {
      "name": "Grace Hopper"
    }
  ],
  "website": "https://example.com/shop?q=1&x=2",
  "require": {
    "example.com/widgets": "v1.4.2"
  },
  "indirect": {
    "golang.org/x/text": "v0.21.0"
  },
  "exclude": {
    "example.com/widgets": [
      "v1.2.0",
      "v1.3.0"
    ]
  },
  "replace": [
    {
      "module": "example.com/old",
      "version": "v1.0.0",
      "with": "example.com/new",
      "withVersion": "v1.2.0"
    },
    {
      "module": "example.com/widgets",
      "with": "../widgets"
    }
  ],
  "retract": [
    {
      "low": "v0.9.0",
      "high": "v0.9.2",
      "rationale": "data-loss bug"
    },
    {
      "version": "v0.8.0",
      "rationale": "published accidentally"
    }
  ],
  "tool": [
    "example.com/lint"
  ],
  "ignore": [
    "./third_party"
  ],
  "godebug": {
    "default": "go1.26"
  }
}
`
	f, err := parseAppJSON("app.json", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The input lists a later exclude and a later retract first.
	// Writing sorts them.
	out, err := formatAppJSON("app.json", f)
	if err != nil {
		t.Fatal(err)
	}
	again, err := parseAppJSON("app.json", out, nil)
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, out)
	}
	out2, err := formatAppJSON("app.json", again)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(out2) {
		t.Fatalf("format is not stable\nfirst:\n%s\nsecond:\n%s", out, out2)
	}
	text := string(out)
	for _, want := range []string{
		`"module": "example.com/shop"`,
		`"name": "Shop"`,
		`"version": "0.1.0"`,
		`"license": "MIT"`,
		`"name": "Grace Hopper"`,
		`"website": "https://example.com/shop?q=1&x=2"`,
		`"example.com/widgets": "v1.4.2"`,
		`"golang.org/x/text": "v0.21.0"`,
		`"with": "../widgets"`,
		`"version": "v0.8.0"`,
		`"rationale": "published accidentally"`,
		`"example.com/lint"`,
		`"./third_party"`,
		`"default": "go1.26"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s\n%s", want, text)
		}
	}
	// Exclude versions and retract intervals are sorted.
	if strings.Index(text, `"v1.2.0"`) > strings.Index(text, `"v1.3.0"`) {
		t.Errorf("exclude versions are not sorted\n%s", text)
	}
	if strings.Index(text, `"version": "v0.8.0"`) > strings.Index(text, `"low": "v0.9.0"`) {
		t.Errorf("retracts are not sorted\n%s", text)
	}
	f.AddRequire("example.com/extra", "v0.3.0")
	out, err = formatAppJSON("app.json", f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"example.com/extra": "v0.3.0"`) || !strings.Contains(string(out), "Ada Lovelace") {
		t.Fatalf("edit dropped a requirement or the author\n%s", out)
	}
}

func TestAppJSONErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "unknown",
			src:  `{"module":"example.com/shop","go":"1.26","licence":"MIT"}`,
			want: `unknown field "licence"`,
		},
		{
			name: "duplicate",
			src:  `{"module":"example.com/shop","module":"example.com/other","go":"1.26"}`,
			want: `duplicate field "module"`,
		},
		{
			name: "both",
			src: `{
  "module": "example.com/shop",
  "go": "1.26",
  "require": {"example.com/widgets": "v1.2.3"},
  "indirect": {"example.com/widgets": "v1.2.3"}
}`,
			want: `appears in require and indirect`,
		},
		{
			name: "version",
			src:  `{"module":"example.com/shop","go":"1.26","version":"v0.1.0"}`,
			want: `version: must be major.minor.patch`,
		},
		{
			name: "module",
			src:  `{"module":"Shop","go":"1.26"}`,
			want: `module: invalid module path "Shop"`,
		},
		{
			name: "toolchain",
			src:  `{"module":"example.com/shop","go":"1.26","toolchain":"go1.26.3"}`,
			want: `unknown field "toolchain"`,
		},
		{
			name: "author",
			src:  `{"module":"example.com/shop","go":"1.26","authors":[{"email":"a@b.c"}]}`,
			want: `authors: name is required`,
		},
		{
			name: "directory version",
			src: `{
  "module": "example.com/shop",
  "go": "1.26",
  "replace": [{"module": "example.com/widgets", "with": "../widgets", "withVersion": "v1.2.3"}]
}`,
			want: `directory ../widgets has a version`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAppJSON("app.json", []byte(tc.src), nil)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}
