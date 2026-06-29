// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAuditFailed wraps a [Sink] write failure surfaced by [Recorder.Record]
// under [FailureRequired]. Match it with errors.Is to distinguish an
// audit-write failure from the underlying authorization decision.
var ErrAuditFailed = errors.New("auth/audit: decision not recorded")

// Decision is a single authorization outcome to be recorded. The deciding
// engines (auth/scope, auth/opa) do not build it; the transport adapter or
// application populates it from the request context, where timing, trace, and
// peer information live. Time is supplied by the caller — the package keeps no
// clock — and free-form string fields should carry stable, low-cardinality
// tokens so the trail stays queryable. Never place raw credentials in any field.
type Decision struct {
	// Time is when the decision was made (caller-supplied; UTC recommended).
	Time time.Time
	// Allowed is the outcome: true for grant, false for denial.
	Allowed bool
	// Subject identifies the principal (e.g. the token sub claim).
	Subject string
	// Action is the action key the decision was keyed on — a gRPC full method,
	// an HTTP route, or any stable identifier.
	Action string
	// ResourceType and ResourceID identify the object acted on for
	// object-level decisions; both empty for action-level decisions.
	ResourceType string
	ResourceID   string
	// RequiredScope is the scope the action demanded, when applicable.
	RequiredScope string
	// Reason is a stable token explaining the outcome — for example
	// "scope_denied", "principal_type_mismatch", "not_registered", or a policy
	// decision id. Optional for grants.
	Reason string
	// Attributes carries extra structured context (tenant, trace id, transport,
	// remote address). Keep keys stable; never store secrets.
	Attributes map[string]string
}

// Sink persists an authorization [Decision]. It is the single consumer-side
// extension point: back it with the project's data/audit dispatcher, a
// structured log stream, or any store. Implementations should be safe for
// concurrent use and should not block the caller for long — prefer an
// asynchronous, buffered sink on hot paths. Returning an error matters only
// under [FailureRequired]; otherwise [Recorder] discards it.
type Sink interface {
	Record(ctx context.Context, d Decision) error
}

// SinkFunc adapts a function to a [Sink].
type SinkFunc func(ctx context.Context, d Decision) error

// Record calls f.
func (f SinkFunc) Record(ctx context.Context, d Decision) error { return f(ctx, d) }

// Recorder applies the recording policy ([PolicyMode]) and failure policy
// ([FailureMode]) around a [Sink]. It holds no mutable state and is safe for
// concurrent use. The zero value is not usable; construct one with
// [NewRecorder]. A nil *Recorder is a valid no-op.
type Recorder struct {
	sink Sink
	opts *options
}

// NewRecorder returns a [Recorder] that writes decisions to sink. A nil sink
// yields a no-op recorder, so callers can wire auditing unconditionally and
// leave it unconfigured.
func NewRecorder(sink Sink, opt ...Option) *Recorder {
	return &Recorder{sink: sink, opts: newOptions(opt...)}
}

// Record submits d to the sink subject to the configured policies:
//
//   - a nil recorder or a nil sink is a no-op returning nil;
//   - under [PolicyDenyOnly] an allowed decision is dropped;
//   - a sink error is returned (wrapping [ErrAuditFailed]) only under
//     [FailureRequired]; under [FailureBestEffort] it is discarded so auditing
//     never turns into a denial-of-service path.
//
// The returned error, when non-nil, is the caller's signal to fail the request
// because it could not be recorded — not an authorization verdict.
func (r *Recorder) Record(ctx context.Context, d Decision) error {
	if r == nil || r.sink == nil {
		return nil
	}
	if d.Allowed && r.opts.policyMode == PolicyDenyOnly {
		return nil
	}
	if err := r.sink.Record(ctx, d); err != nil {
		if r.opts.failureMode == FailureRequired {
			return fmt.Errorf("%w: %w", ErrAuditFailed, err)
		}
	}
	return nil
}
