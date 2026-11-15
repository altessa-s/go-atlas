// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import "log/slog"

//go:generate go tool optgen generate

type options struct {
	// rollbackOnFailure rolls back all applied migrations when an up step fails.
	rollbackOnFailure bool
	// logger receives rollback and lease-release failures.
	logger *slog.Logger
}
