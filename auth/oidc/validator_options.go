// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "log/slog"

// validatorOptions holds configuration for a [Validator].
type validatorOptions struct {
	// logger receives an error record for every failed verification.
	logger *slog.Logger
}
