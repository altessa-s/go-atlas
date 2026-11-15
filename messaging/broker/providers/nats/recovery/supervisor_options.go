// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=supervisorOptions --option-type=SupervisorOption --option-prefix=Supervisor --output=supervisor_options_gen.go

import (
	"log/slog"
	"time"
)

// supervisorOptions configures [Supervisor]. All fields are populated by generated [SupervisorOption] setters in
// supervisor_options_gen.go.
type supervisorOptions struct {
	// logger receives recovery progress and failures.
	logger *slog.Logger

	// maxAttempts is the maximum number of recovery attempts before giving up.
	maxAttempts int `optgen:"default=DefaultMaxRecoveryAttempts" optval:"positive"`

	// backoff is the base delay between attempts; attempt n waits backoff * n.
	backoff time.Duration `optgen:"default=DefaultRecoveryBackoff"`

	// onRecoverySuccess is called after a stream or consumer is recovered.
	onRecoverySuccess RecoverySuccessCallback

	// onRecoveryFailure is called when recovery fails after all attempts.
	onRecoveryFailure RecoveryFailureCallback

	// onManualNeeded is called for streams configured with [RecoveryStrategyManual].
	onManualNeeded ManualRecoveryCallback

	// onStaleCleared is called when a stale recovery mark is cleared.
	onStaleCleared StaleRecoveryClearedCallback
}
