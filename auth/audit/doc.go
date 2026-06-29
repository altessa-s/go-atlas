// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package audit records authorization decisions — who was allowed or denied
// which action on which resource, and why — as durable, structured, queryable
// events, distinct from aggregate metrics and ephemeral logs. It is the audit
// trail compliance regimes (SOC 2, ISO 27001, PCI, HIPAA) require for access
// control, and the forensic record for "who could touch resource X, and when".
//
// # Model
//
// A [Decision] is the event: the outcome, the principal, the action, the
// optional resource, and the reason. A [Sink] is the single consumer-side seam
// that persists it — back this with the project's data/audit dispatcher, a log
// stream, or any store; the package ships no sink and no I/O of its own. A
// [Recorder] wraps a Sink with two policies and is the type callers hold.
//
// The core decision engines (auth/scope, auth/opa) stay pure and unaware of
// auditing; the transport adapter or application builds a [Decision] from the
// request context — where the trace id, peer, and timing live — and hands it to
// a [Recorder].
//
// # Policies
//
// [PolicyMode] selects what is recorded: [PolicyDenyOnly] (the default — every
// denial, no allows, keeping volume bounded) or [PolicyAll].
//
// [FailureMode] selects what a sink error means: [FailureBestEffort] (the
// default — a failed write never blocks the request; auditing must not become a
// denial-of-service vector) or [FailureRequired] ("no audit, no action" — the
// write error is returned wrapping [ErrAuditFailed], for regimes that must not
// act unrecorded).
//
// A nil [Recorder] and a Recorder with a nil Sink are both valid no-ops, so a
// caller can wire auditing unconditionally and leave it unconfigured.
//
// # Usage
//
//	rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))
//
//	// In a transport auth adapter, after the scope/opa decision:
//	err := enf.Enforce(principal, key)
//	_ = rec.Record(ctx, audit.Decision{
//	    Time:          now,
//	    Allowed:       err == nil,
//	    Subject:       principal.Subject,
//	    Action:        key,
//	    RequiredScope: required,
//	    Reason:        reasonOf(err),
//	    Attributes:    map[string]string{"trace_id": traceID, "tenant": tenant},
//	})
package audit
