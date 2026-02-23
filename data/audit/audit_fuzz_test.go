// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func FuzzAuditor_Emit(f *testing.F) {
	f.Add("api.request", "create", "user", "u1", "order", "o1")
	f.Add("", "", "", "", "", "")
	f.Add("data.change", "update", "service", "svc-1", "document", "doc-xyz")

	f.Fuzz(func(t *testing.T, eventType, action, actorType, actorID, resType, resID string) {
		store := memory.New()
		a, err := audit.New(store, audit.WithFlushInterval(50*time.Millisecond), audit.WithWorkers(1))
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Start(); err != nil {
			t.Fatal(err)
		}

		event := &audit.Event{
			Type:   audit.EventType(eventType),
			Action: audit.Action(action),
			Actor:  audit.Actor{Type: audit.ActorType(actorType), ID: actorID},
			Resource: audit.Resource{
				Type: resType,
				ID:   resID,
			},
			Result: audit.Result{Status: audit.ResultStatusSuccess},
		}
		a.Emit(event)
		_ = a.Shutdown(t.Context())
	})
}

func FuzzMemoryStorage_MatchesQuery(f *testing.F) {
	f.Add("u1", "", "order", "", "success", "", 10, 0)
	f.Add("", "user", "", "o1", "", "req-1", 0, 0)

	f.Fuzz(func(t *testing.T, actorID, actorType, resType, resID, status, requestID string, limit, offset int) {
		store := memory.New()
		ctx := t.Context()

		_ = store.Store(ctx, &audit.Event{
			Type:   audit.EventTypeAPIRequest,
			Action: audit.ActionRead,
			Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
			Resource: audit.Resource{
				Type: "order",
				ID:   "o1",
			},
			Result:  audit.Result{Status: audit.ResultStatusSuccess},
			Context: audit.EventContext{RequestID: "req-1"},
		})

		q := &audit.Query{
			ActorID:      actorID,
			ActorType:    actorType,
			ResourceType: resType,
			ResourceID:   resID,
			Status:       audit.ResultStatus(status),
			RequestID:    requestID,
			Limit:        limit,
			Offset:       offset,
		}

		// Should not panic
		for range store.Query(ctx, q) {
		}
		_, _ = store.Count(ctx, q)
	})
}
