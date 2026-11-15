// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/audit"
)

func TestBuildTransportEvent(t *testing.T) {
	start := time.Now()
	info := audit.RequestInfo{
		Action:       audit.ActionRead,
		ResourceType: "http.endpoint",
		ResourcePath: "/api/v1/items",
		Actor: audit.Actor{
			Type: audit.ActorTypeUser,
			ID:   "user-1",
			IP:   "1.2.3.4",
		},
		Context:   audit.NewEventContext("trace-1", "span-1", "req-1"),
		StartTime: start,
		Duration:  100 * time.Millisecond,
	}

	event := audit.BuildTransportEvent(info, func() audit.Result {
		return audit.Result{Status: audit.ResultStatusSuccess, Code: 200}
	})

	assert.Equal(t, audit.EventTypeAPIRequest, event.Type)
	assert.Equal(t, audit.ActionRead, event.Action)
	assert.Equal(t, "http.endpoint", event.Resource.Type)
	assert.Equal(t, "/api/v1/items", event.Resource.Path)
	assert.Equal(t, "user-1", event.Actor.ID)
	assert.Equal(t, audit.ResultStatusSuccess, event.Result.Status)
	assert.Equal(t, 200, event.Result.Code)
	assert.Equal(t, "trace-1", event.Context.TraceID)
	assert.Equal(t, "span-1", event.Context.SpanID)
	assert.Equal(t, "req-1", event.Context.RequestID)
	assert.Equal(t, start, event.Timestamp)
	assert.Equal(t, 100*time.Millisecond, event.Duration)
}

func TestNewEventContext(t *testing.T) {
	ctx := audit.NewEventContext("trace-abc", "span-def", "req-ghi")

	assert.Equal(t, "trace-abc", ctx.TraceID)
	assert.Equal(t, "span-def", ctx.SpanID)
	assert.Equal(t, "req-ghi", ctx.RequestID)
}
