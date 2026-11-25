// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package rules checks CEL expressions, clock skew, and token lifetime bounds for OIDC validation policies.
//
// # CEL expressions
//
// CompileCEL checks the expression type and installs the evaluation cost limit.
//
//	program, err := rules.CompileCEL("claims.sub == 'service'")
package rules
