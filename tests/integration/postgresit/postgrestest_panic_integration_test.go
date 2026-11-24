// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

// The fixture uses pgx directly. Interrupt its first cancellable wire operation at the selected
// setup stage, after database/sql has checked out the working connection. Each test owns its trigger.
type panicContext struct {
	context.Context
	armed atomic.Bool
}

func (c *panicContext) Done() <-chan struct{} {
	if c.armed.Load() {
		var callers [32]uintptr
		n := runtime.Callers(2, callers[:])
		frames := runtime.CallersFrames(callers[:n])
		for {
			f, more := frames.Next()
			if strings.HasPrefix(f.Function, "github.com/jackc/pgx/v5/pgconn.") && c.armed.Swap(false) {
				panic("fixture setup interrupted")
			}
			if !more {
				break
			}
		}
	}
	return c.Context.Done()
}

type panicTB struct {
	testing.TB
	ctx                  *panicContext
	after, registrations int
}

func (p *panicTB) Context() context.Context { return p.ctx }
func (p *panicTB) Cleanup(f func()) {
	p.TB.Cleanup(f)
	p.registrations++
	if p.registrations == p.after {
		if p.after == 2 {
			panic("fixture setup interrupted")
		} // the confirmed marker now owns a cleanup
		p.ctx.armed.Store(true)
	}
}

func testPanicCleanup(t *testing.T, after int) {
	t.Helper()
	admin := openDatabase(t, "postgres")
	prefix := fixturePrefix()
	done := make(chan bool, 1)
	go func() {
		done <- t.Run("setup", func(t *testing.T) {
			tb := &panicTB{TB: t, after: after, ctx: &panicContext{Context: t.Context()}}
			require.PanicsWithValue(t, "fixture setup interrupted", func() { postgrestest.New(tb, postgrestest.WithDatabasePrefix(prefix)) })
		})
	}()
	select {
	case ok := <-done:
		require.True(t, ok)
	case <-time.After(time.Minute):
		t.Fatal("fixture cleanup blocked")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	names, err := databaseNames(ctx, admin, prefix)
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestPostgrestest_PanicInsideNewLeaksNothing(t *testing.T) {
	t.Parallel()
	// Setup registers the admin connection, marked database, target lease, then working pool.
	for _, tc := range []struct {
		name  string
		after int
	}{{"maintenance connection", 1}, {"marked database", 2}, {"working pool", 4}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testPanicCleanup(t, tc.after)
		})
	}
}
