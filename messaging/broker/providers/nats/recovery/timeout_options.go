// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=recoveryTimeoutMonitorOptions --option-type=RecoveryTimeoutMonitorOption --option-prefix=RecoveryTimeoutMonitor --output=timeout_options_gen.go

import (
	"log/slog"
	"time"
)

// recoveryTimeoutMonitorOptions configures [RecoveryTimeoutMonitor]. All fields are populated by generated
// [RecoveryTimeoutMonitorOption] setters in timeout_options_gen.go.
type recoveryTimeoutMonitorOptions struct {
	// logger receives stale-mark cleanup reports.
	logger *slog.Logger

	// timeout is how long a recovery mark may exist before it is considered stale.
	timeout time.Duration `optgen:"default=DefaultStaleRecoveryTimeout"`
}
