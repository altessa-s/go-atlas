// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dataaudit

import (
	"context"
	"errors"

	authaudit "github.com/altessa-s/go-atlas/auth/audit"
	coreaudit "github.com/altessa-s/go-atlas/data/audit"
)

// ErrDropped is returned by [Sink.Record] when the underlying auditor dropped
// the event (a full buffer). Under audit.FailureRequired it propagates so the
// caller can fail a request that could not be recorded.
var ErrDropped = errors.New("auth/audit/sinks/dataaudit: audit event dropped")

// Sink adapts a data/audit *Auditor to the [authaudit.Sink] seam, emitting each
// authorization [authaudit.Decision] as a data/audit Event.
type Sink struct {
	auditor *coreaudit.Auditor
}

// New returns a [Sink] that emits decisions through auditor, which must already
// be started. A nil auditor yields a no-op sink.
func New(auditor *coreaudit.Auditor) *Sink {
	return &Sink{auditor: auditor}
}

// Record maps d to a data/audit Event of type EventTypeAuth and emits it. It
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

	ev := s.auditor.NewEvent(coreaudit.EventTypeAuth, coreaudit.ActionExecute).
		WithActor(coreaudit.Actor{ID: d.Subject}).
		WithResource(coreaudit.Resource{Type: d.ResourceType, ID: d.ResourceID, Path: d.Action}).
		WithResult(result).
		WithMetadata(meta).
		Build()
	if !d.Time.IsZero() {
		ev.Timestamp = d.Time
	}

	if !s.auditor.Emit(ev) {
		return ErrDropped
	}
	return nil
}

// Sink satisfies the authorization audit seam.
var _ authaudit.Sink = (*Sink)(nil)
