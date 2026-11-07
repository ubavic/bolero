// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2_test

import "testing"

func TestBolero(t *testing.T) {
	testDirFiles(t, "../../../../internal/types/testdata/bolero", 0, false)
}
