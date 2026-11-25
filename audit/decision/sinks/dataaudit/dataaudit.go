// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dataaudit

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	coreaudit "github.com/altessa-s/go-atlas/audit"
	authaudit "github.com/altessa-s/go-atlas/audit/decision"
)

// ErrDropped is returned by [Sink.Record] when the underlying auditor dropped
// the event (a full buffer). Under audit.FailureRequired it propagates so the
// caller can fail a request that could not be recorded.
var ErrDropped = errors.New("audit/decision/sinks/dataaudit: audit event dropped")

// Emitter accepts an audit event and reports whether it was queued.
type Emitter interface {
	Emit(*coreaudit.Event) bool
}

// Sink emits authorization decisions as audit events.
type Sink struct{ auditor Emitter }

// New returns a [Sink] that emits decisions through auditor, which must already
// be started. A nil auditor yields a no-op sink.
func New(auditor Emitter) *Sink {
	if nilcheck.IsNil(auditor) {
		return &Sink{}
	}
	return &Sink{auditor: auditor}
}

// Record maps d to a audit Event of type EventTypeAuth and emits it. It
// returns [ErrDropped] if the auditor dropped the event, and nil otherwise
// (including for the no-op nil cases).
func (s *Sink) Record(_ context.Context, d authaudit.Decision) error {
	if s == nil || s.auditor == nil {
		return nil
	}

	result := coreaudit.Result{Status: coreaudit.ResultStatusSuccess}
	if !d.Allowed {
		result = coreaudit.Result{Status: coreaudit.ResultStatusDenied, Message: d.Reason}
	}

	var meta map[string]any
	if d.RequiredScope != "" || len(d.Attributes) > 0 {
		meta = make(map[string]any, len(d.Attributes)+1)
		for k, v := range d.Attributes {
			meta[k] = v
		}
		if d.RequiredScope != "" {
			meta["required_scope"] = d.RequiredScope
		}
	}

	ev := &coreaudit.Event{
		Type:      coreaudit.EventTypeAuth,
		Action:    coreaudit.ActionExecute,
		Actor:     coreaudit.Actor{ID: d.Subject},
		Resource:  coreaudit.Resource{Type: d.ResourceType, ID: d.ResourceID, Path: d.Action},
		Result:    result,
		Metadata:  meta,
		Timestamp: d.Time,
	}

	if !s.auditor.Emit(ev) {
		return ErrDropped
	}
	return nil
}

// Sink satisfies the authorization audit seam.
var _ authaudit.Sink = (*Sink)(nil)
