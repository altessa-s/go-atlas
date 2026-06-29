// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// PolicyMode selects which decisions a [Recorder] records.
type PolicyMode int

const (
	// PolicyDenyOnly records only denials. It is the default: it captures the
	// security-relevant events while keeping volume bounded.
	PolicyDenyOnly PolicyMode = iota
	// PolicyAll records both grants and denials — a full access trail, at the
	// cost of one sink write per authorized request.
	PolicyAll
)

// String implements [fmt.Stringer].
func (m PolicyMode) String() string {
	switch m {
	case PolicyDenyOnly:
		return "deny_only"
	case PolicyAll:
		return "all"
	default:
		return "unknown"
	}
}

// FailureMode selects what a [Sink] write error means.
type FailureMode int

const (
	// FailureBestEffort discards sink errors: a failed audit write never blocks
	// the request. It is the default — auditing must not become a
	// denial-of-service vector.
	FailureBestEffort FailureMode = iota
	// FailureRequired propagates sink errors (wrapping [ErrAuditFailed]) so the
	// caller can fail a request that could not be recorded — "no audit, no
	// action", for regimes that must never act unrecorded.
	FailureRequired
)

// String implements [fmt.Stringer].
func (m FailureMode) String() string {
	switch m {
	case FailureBestEffort:
		return "best_effort"
	case FailureRequired:
		return "required"
	default:
		return "unknown"
	}
}

// Default recording and failure policies.
const (
	// DefaultPolicyMode records only denials.
	DefaultPolicyMode = PolicyDenyOnly
	// DefaultFailureMode never lets a failed audit write block a request.
	DefaultFailureMode = FailureBestEffort
)

// options carries the recorder tunables.
type options struct {
	policyMode  PolicyMode  `optgen:"default=DefaultPolicyMode"`
	failureMode FailureMode `optgen:"default=DefaultFailureMode"`
}
