// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cfgerrors classifies configuration errors raised outside the
// ozzo-validation rules, so callers match them with errors.Is instead of by
// message text. Internal package; API may change without notice.
//
// Example:
//
//	return cfgerrors.Invalidf("username must not be set when connectionURI is used")
package cfgerrors
