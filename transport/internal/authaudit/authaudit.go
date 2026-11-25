// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authaudit

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/audit/decision"
)

// Record emits a decision with the transport that handled the request.
// A nil recorder disables auditing.
func Record(ctx context.Context, recorder decision.Sink, transport, action, subject string, allowed bool, reason string) error {
	if recorder == nil {
		return nil
	}
	return recorder.Record(ctx, decision.Decision{
		Time:       time.Now().UTC(),
		Allowed:    allowed,
		Subject:    subject,
		Action:     action,
		Reason:     reason,
		Attributes: map[string]string{"transport": transport},
	})
}

// RecordAuthentication extracts the subject of a successful authentication when auditing is enabled.
func RecordAuthentication[P any](
	ctx context.Context, recorder decision.Sink, transport, action string,
	principal P, subjectOf func(P) string, allowed bool, reason string,
) error {
	if recorder == nil {
		return nil
	}
	subject := ""
	if allowed && subjectOf != nil {
		subject = subjectOf(principal)
	}
	return Record(ctx, recorder, transport, action, subject, allowed, reason)
}
