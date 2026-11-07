// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modindex

import (
	"go/build"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestIndexBoleroFiles(t *testing.T) {
	src := filepath.Join(runtime.GOROOT(), "src")
	const pkg = "go/build/testdata/bolero"

	m, err := fromBytes(src, encodeModuleBytes([]*rawPackage{importRaw(src, pkg)}))
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := m.Package(pkg).Import(build.Default, build.ImportComment)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := build.Default.Import(".", filepath.Join(src, pkg), build.ImportComment)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(indexed, direct) {
		t.Errorf("index and go/build disagree:\nindex:    %+v\ngo/build: %+v", indexed, direct)
	}
	if len(indexed.GoFiles) != 2 || len(indexed.TestGoFiles) != 1 {
		t.Errorf("GoFiles = %q, TestGoFiles = %q, want a.go, b.bo and b_test.bo", indexed.GoFiles, indexed.TestGoFiles)
	}
}
