// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"internal/testenv"
	"path/filepath"
	"testing"
)

// boleroFeatures returns the API added by Bolero, listed in api/bolero/*.txt.
// Bolero API needs no proposal approval, so unlike api/next/*.txt it is
// read for release versions too.
func boleroFeatures(t *testing.T) []string {
	files, err := filepath.Glob(filepath.Join(testenv.GOROOT(t), "api/bolero/*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var features []string
	for _, file := range files {
		features = append(features, fileFeatures(file, false)...)
	}
	return features
}
