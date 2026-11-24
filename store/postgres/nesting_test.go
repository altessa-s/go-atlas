// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
	"github.com/altessa-s/go-atlas/store/postgres"
)

// With develop's single transaction binding, B cannot start on A's context.
// Independently started roots can finish in either order, and neither can re-enter the other.
func TestNesting(t *testing.T) {
	t.Parallel()
	for bits := range 64 {
		aRunner, bRunner, reenter, aFirst, aCommit, bCommit := bits&1 != 0, bits&2 != 0, bits&4 != 0, bits&8 != 0, bits&16 != 0, bits&32 != 0
		t.Run(fmt.Sprintf("runners=%t,%t/reenter=%t/aFirst=%t/commit=%t,%t", aRunner, bRunner, reenter, aFirst, aCommit, bCommit), func(t *testing.T) {
			t.Parallel()
			type side struct {
				c                 *postgres.Committer
				runner            *uow.Runner
				fake              *testhelpers.FakeSQL
				run               func(context.Context, func(context.Context) error) error
				viaRunner, commit bool
				hooks             int
			}
			newSide := func(viaRunner, commit bool) *side {
				db, fake := testhelpers.NewFakeSQL(t, nil)
				c, err := postgres.NewCommitter(db)
				require.NoError(t, err)
				r := uow.New(c, nil, uow.WithDetector(postgres.Transaction))
				s := &side{c: c, runner: r, fake: fake, run: c.WithTransaction, viaRunner: viaRunner, commit: commit}
				if viaRunner {
					s.run = r.Run
				}
				return s
			}
			a, b := newSide(aRunner, aCommit), newSide(bRunner, bCommit)
			rollback := errors.New("rollback")
			type active struct {
				ctx    context.Context
				finish func()
			}
			start := func(s *side) active {
				ready := make(chan context.Context, 1)
				release := make(chan struct{})
				releaseBody := sync.OnceFunc(func() { close(release) })
				t.Cleanup(releaseBody)
				done := make(chan error, 1)
				go func() {
					done <- s.run(t.Context(), func(ctx context.Context) error {
						ready <- ctx
						<-release
						if s.commit {
							return nil
						}
						return rollback
					})
				}()
				var ctx context.Context
				select {
				case ctx = <-ready:
				case err := <-done:
					t.Fatalf("body never started: %v", err)
				}
				return active{ctx: ctx, finish: func() {
					releaseBody()
					err := <-done
					if s.commit {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, rollback)
					}
					_, open := s.c.Detect(ctx)
					require.False(t, open)
				}}
			}
			as, bs := start(a), start(b)
			check := func(ctx context.Context, owner, other *side) {
				id, ok := owner.c.Detect(ctx)
				require.True(t, ok)
				_, ok = other.c.Detect(ctx)
				require.False(t, ok)
				require.NoError(t, owner.c.WithTransaction(ctx, func(inner context.Context) error {
					nested, ok := owner.c.Detect(inner)
					require.True(t, ok)
					require.Same(t, id, nested)
					return nil
				}))
				err := owner.runner.Run(ctx, func(ctx context.Context) error {
					return owner.runner.AfterCommit(ctx, func(context.Context) { require.Equal(t, 1, owner.fake.Commits()); owner.hooks++ })
				})
				if owner.viaRunner {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, uow.ErrForeignTransaction)
				}
				err = other.run(context.WithoutCancel(ctx), func(context.Context) error { t.Fatal("foreign body ran"); return nil })
				if other.viaRunner {
					require.ErrorIs(t, err, uow.ErrForeignTransaction)
				} else {
					require.ErrorIs(t, err, postgres.ErrDifferentDatabase)
				}
			}
			check(as.ctx, a, b)
			if reenter {
				check(bs.ctx, b, a)
			}
			if aFirst {
				as.finish()
				check(bs.ctx, b, a)
				bs.finish()
			} else {
				bs.finish()
				check(as.ctx, a, b)
				as.finish()
			}
			for _, s := range []*side{a, b} {
				want := 0
				if s.viaRunner && s.commit {
					if s == a {
						want = 1
						if !aFirst {
							want++
						}
					} else {
						if reenter {
							want++
						}
						if aFirst {
							want++
						}
					}
				}
				require.Equal(t, want, s.hooks)
			}
		})
	}
}
