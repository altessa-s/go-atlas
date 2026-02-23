// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package guard provides early-return guard plugins for optgen.
//
// Guards run first in the option pipeline and can short-circuit execution.
// The notnil guard is applied by default to pointer and interface types.
//
// # Tag Usage
//
//	type Options struct {
//	    Handler func() `optgen:"notnil"`   // default for pointers/interfaces
//	    Data    *Data  `optgen:"allownull"` // disable notnil guard
//	}
package guard
