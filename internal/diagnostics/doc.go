// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package diagnostics sanitizes error text for logs, spans and persisted outcomes.
//
// # Transport errors
//
// Use ErrorClass for external component failures and ClassifiedError at
// callback boundaries where callers need to match sentinels.
//
//	logger.Warn("write failed", "classification", diagnostics.ErrorClass(err))
package diagnostics
