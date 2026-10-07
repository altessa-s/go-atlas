// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package claimscope normalizes a JWT scope claim value shared by the auth
// packages that read scopes from decoded claims.
//
// # Accepted shapes
//
// A space-separated string (OAuth 2.0 "scope"), a []string, or a []any whose
// string elements are kept and other elements skipped. Any other type yields
// nothing.
//
//	for s := range claimscope.Seq(claims["scope"]) {
//		// ...
//	}
package claimscope
