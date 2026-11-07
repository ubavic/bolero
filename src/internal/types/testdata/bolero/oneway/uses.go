// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Go source files may not use declarations from Bolero source files,
// even within the same package.

package oneway

type GoType struct{}

func (GoType) GoMethod() {}

func GoFunc() {}

var GoVar int

func _() {
	GoFunc()
	GoType{}.GoMethod()

	BoFunc /* ERROR "BoFunc is declared in a Bolero file and cannot be used in Go files" */ ()
	var _ BoType /* ERROR "BoType is declared in a Bolero file and cannot be used in Go files" */
	_ = BoVar /* ERROR "BoVar is declared in a Bolero file and cannot be used in Go files" */
	_ = BoConst /* ERROR "BoConst is declared in a Bolero file and cannot be used in Go files" */
	GoType{}.BoMethod /* ERROR "BoMethod is declared in a Bolero file and cannot be used in Go files" */ ()
	_ = GoType.BoMethod /* ERROR "BoMethod is declared in a Bolero file and cannot be used in Go files" */
}

func (BoType /* ERROR "BoType is declared in a Bolero file and cannot be used in Go files" */) M() {}
