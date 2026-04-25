// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

// RecoveryStrategy defines how the system responds when a stream or consumer is deleted.
type RecoveryStrategy int

const (
	// RecoveryStrategyAuto enables automatic recovery with exponential backoff.
	// When deletion is detected, the system automatically recreates the stream
	// and consumers from saved configuration. This is the default strategy.
	//
	// Use for critical streams that must be restored automatically.
	RecoveryStrategyAuto RecoveryStrategy = iota

	// RecoveryStrategyManual notifies via the OnManualRecoveryNeeded callback
	// but does not attempt automatic recovery. Useful for streams that require
	// human review before restoration (e.g., streams with special configuration,
	// external dependencies, or compliance requirements).
	RecoveryStrategyManual

	// RecoveryStrategySkip ignores deletion events entirely. No recovery attempt
	// is made and no notification is sent.
	//
	// Use for:
	//   - Ephemeral streams (temporary data, acceptable loss)
	//   - Externally-managed streams (created/managed by another service)
	//   - Streams that should not exist after deletion (intentional cleanup)
	RecoveryStrategySkip
)

// String returns a string representation of the recovery strategy.
func (s RecoveryStrategy) String() string {
	switch s {
	case RecoveryStrategyAuto:
		return "auto"
	case RecoveryStrategyManual:
		return "manual"
	case RecoveryStrategySkip:
		return "skip"
	default:
		return "unknown"
	}
}

// IsAuto returns true if the strategy is automatic recovery.
func (s RecoveryStrategy) IsAuto() bool {
	return s == RecoveryStrategyAuto
}

// IsManual returns true if the strategy is manual recovery.
func (s RecoveryStrategy) IsManual() bool {
	return s == RecoveryStrategyManual
}

// IsSkip returns true if the strategy is to skip recovery.
func (s RecoveryStrategy) IsSkip() bool {
	return s == RecoveryStrategySkip
}
