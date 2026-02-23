// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package constraints provides reusable generic constraints for numbers, primitives, and strings.
// Use them to simplify generic function bounds.
//
// Example:
//
//	func Sum[T constraints.Numbers](vals []T) T {
//	    var total T
//	    for _, v := range vals {
//	        total += v
//	    }
//	    return total
//	}
package constraints
