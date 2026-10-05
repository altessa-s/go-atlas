// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// recordingScheduler keeps every registered task and fails registration of
// the task whose ID equals failID.
type recordingScheduler struct {
	failID string
	mu     sync.Mutex
	tasks  map[string]corescheduler.TaskFunc
}

var errRegister = errors.New("register failed")

func (s *recordingScheduler) Register(_ context.Context, cfg corescheduler.TaskConfig) error {
	if cfg.ID == s.failID {
		return errRegister
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks == nil {
		s.tasks = map[string]corescheduler.TaskFunc{}
	}
	s.tasks[cfg.ID] = cfg.Func
	return nil
}

func (s *recordingScheduler) task(id string) corescheduler.TaskFunc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[id]
}

// syncStorage is a RevocationStorage whose Sync returns err and counts calls.
type syncStorage struct {
	err   error
	syncs atomic.Int32
}

func (s *syncStorage) IsRevoked(context.Context, string) (bool, error)          { return false, nil }
func (s *syncStorage) MarkRevoked(context.Context, string, time.Duration) error { return nil }
func (s *syncStorage) Sync(context.Context) error {
	s.syncs.Add(1)
	return s.err
}

// A scheduler registration failure after an earlier task was registered must
// leave that task inert: construction failed, so the provider is closed and
// the orphaned task must not keep refreshing JWKS against it.
func TestNewProvider_FailedSchedulerRegistrationClosesProvider(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	sched := &recordingScheduler{failID: "oidc-revocation-sync"}
	storage := &syncStorage{}

	_, err := idp.tryNewProvider(t,
		WithScheduler(sched),
		WithJWKSRefreshSchedule("0 */30 * * * *"),
		WithRevocationStorage(storage),
		WithRevocationSyncSchedule("0 */5 * * * *"),
	)
	require.ErrorIs(t, err, errRegister)

	jwksTask := sched.task("oidc-jwks-refresh")
	require.NotNil(t, jwksTask, "precondition: the JWKS task was registered before the failure")

	err = jwksTask(t.Context())
	require.ErrorIs(t, err, context.Canceled, "the background context of a failed provider must be canceled")
}

// Scheduled tasks of a closed provider no-op.
func TestProvider_CloseDisarmsScheduledTasks(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	sched := &recordingScheduler{}
	storage := &syncStorage{}

	p, err := idp.tryNewProvider(t,
		WithScheduler(sched),
		WithRevocationStorage(storage),
		WithRevocationSyncSchedule("0 */5 * * * *"),
	)
	require.NoError(t, err)
	syncTask := sched.task("oidc-revocation-sync")
	require.NotNil(t, syncTask)

	require.NoError(t, syncTask(t.Context()))
	require.EqualValues(t, 2, storage.syncs.Load(), "initial sync plus one scheduled run")

	p.Close()
	require.ErrorIs(t, syncTask(t.Context()), context.Canceled)
	require.EqualValues(t, 2, storage.syncs.Load(), "a closed provider must not sync")
}

// TestNewProvider_FailedConstructionDoesNotLeakGoroutines is deliberately
// serial: it compares the process-wide goroutine count, which parallel tests
// would perturb (parallel tests only start once all serial tests finished).
func TestNewProvider_FailedConstructionDoesNotLeakGoroutines(t *testing.T) {
	idp := newTestIdP(t)

	tests := []struct {
		name string
		opts func() []Option
	}{
		{name: "initial revocation sync fails", opts: func() []Option {
			return []Option{WithRevocationStorage(&syncStorage{err: errors.New("sync failed")})}
		}},
		{name: "scheduler registration fails", opts: func() []Option {
			return []Option{
				WithScheduler(&recordingScheduler{failID: "oidc-jwks-refresh"}),
				WithJWKSRefreshSchedule("0 */30 * * * *"),
			}
		}},
		{name: "JWKS initialization fails", opts: func() []Option {
			idp.jwksStatus.Store(http.StatusInternalServerError)
			return nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { idp.jwksStatus.Store(http.StatusOK) })
			baseline := runtime.NumGoroutine()

			for range 5 {
				_, err := idp.tryNewProvider(t, tc.opts()...)
				require.Error(t, err)
			}

			testhelpers.WaitFor(t, 2*time.Second, func() bool {
				return runtime.NumGoroutine() <= baseline
			}, "failed NewProvider calls leaked goroutines")
		})
	}
}
