// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func FuzzStorage_Query(f *testing.F) {
	f.Add("u1", "", "", "", 10, 0)
	f.Add("", "user", "order", "success", 0, 0)
	f.Add("", "", "", "", 5, 2)

	f.Fuzz(func(t *testing.T, actorID, actorType, resType, status string, limit, offset int) {
		s := memory.New()
		ctx := t.Context()

		_ = s.Store(ctx, &audit.Event{
			Actor:     audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
			Resource:  audit.Resource{Type: "order", ID: "o1"},
			Result:    audit.Result{Status: audit.ResultStatusSuccess},
			Timestamp: time.Now(),
		})

		q := &audit.Query{
			ActorID:      actorID,
			ActorType:    actorType,
			ResourceType: resType,
			Status:       audit.ResultStatus(status),
			Limit:        limit,
			Offset:       offset,
		}

		// Should not panic
		for range s.Query(ctx, q) {
		}
		_, _ = s.Count(ctx, q)
	})
}
