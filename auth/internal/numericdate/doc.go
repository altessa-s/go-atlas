// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package numericdate reads exact JWT dates for signing, verification and OIDC policy.
//
// # Comparison
//
// Compare the difference between two dates directly to a duration:
//
//	exp, ok := numericdate.Parse(claims["exp"])
//	expired := ok && exp.CompareOffset(numericdate.FromTime(now), -leeway) <= 0
package numericdate
