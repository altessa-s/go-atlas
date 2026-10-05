// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// refreshEvery is the shortest interval robfig/cron supports.
const refreshEvery = "@every 1s"

// syncStorage is a RevocationStorage whose Sync returns err (or, when script
// is set, script[i] for the i-th call, repeating the last entry) and counts
// calls.
type syncStorage struct {
	err    error
	script []error
	syncs  atomic.Int32
}

func (s *syncStorage) IsRevoked(context.Context, string) (bool, error)          { return false, nil }
func (s *syncStorage) MarkRevoked(context.Context, string, time.Duration) error { return nil }
func (s *syncStorage) Sync(context.Context) error {
	n := int(s.syncs.Add(1))
	if len(s.script) > 0 {
		return s.script[min(n, len(s.script))-1]
	}
	return s.err
}

func TestNewProvider_InvalidRefreshScheduleFails(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)

	_, err := idp.tryNewProvider(t, WithJWKSRefreshSchedule("not a cron"))
	require.ErrorContains(t, err, "invalid jwks-refresh schedule")

	_, err = idp.tryNewProvider(t, WithRevocationStorage(&syncStorage{}), WithRevocationSyncSchedule("61 * * * * *"))
	require.ErrorContains(t, err, "invalid revocation-sync schedule")
}

// Each provider owns its cron: two providers in one process both keep
// refreshing (no shared task IDs that could replace each other).
func TestRefreshCron_TwoProvidersRefreshIndependently(t *testing.T) {
	t.Parallel()

	idpA, idpB := newTestIdP(t), newTestIdP(t)
	storageA, storageB := &syncStorage{}, &syncStorage{}
	idpA.newProvider(t, WithJWKSRefreshSchedule(refreshEvery),
		WithRevocationStorage(storageA), WithRevocationSyncSchedule(refreshEvery))
	idpB.newProvider(t, WithJWKSRefreshSchedule(refreshEvery),
		WithRevocationStorage(storageB), WithRevocationSyncSchedule(refreshEvery))

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		// Construction fetched each JWKS and synced each storage once.
		return idpA.jwksRequests.Load() >= 2 && idpB.jwksRequests.Load() >= 2 &&
			storageA.syncs.Load() >= 2 && storageB.syncs.Load() >= 2
	}, "both providers must run their own JWKS refresh and revocation sync")
}

// A revocation added to the source after startup reaches the filter through
// the provider's cron, without any external scheduler.
func TestRefreshCron_RevocationSyncPicksUpSourceChange(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	raw := idp.sign(t, idp.claims(nil))
	p := idp.newProvider(t,
		WithRevocationFilter(newRebuildableFilter()),
		WithRevocationLoader(&URLRevocationLoader{URL: idp.srv.URL + "/revoked"}),
		WithRevocationSyncSchedule(refreshEvery),
	)

	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)

	idp.setRevoked(raw)
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_, err := p.ValidateToken(t.Context(), raw)
		return errors.Is(err, ErrTokenRevoked)
	}, "the scheduled sync must pick up the new revocation")
}

func TestProvider_CloseStopsRefreshCron(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t, WithJWKSRefreshSchedule(refreshEvery))
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return idp.jwksRequests.Load() >= 2 },
		"precondition: the cron refreshes the JWKS")

	p.Close()
	after := idp.jwksRequests.Load()
	time.Sleep(2500 * time.Millisecond) // more than two cron periods
	require.Equal(t, after, idp.jwksRequests.Load(), "a closed provider's cron must not refresh")
}

// TestProvider_NoGoroutineLeaks is deliberately serial: it compares the
// process-wide goroutine count, which parallel tests would perturb (parallel
// tests only start once all serial tests finished).
func TestProvider_NoGoroutineLeaks(t *testing.T) {
	idp := newTestIdP(t)

	tests := []struct {
		name    string
		opts    func() []Option
		wantErr bool
	}{
		{name: "closed provider with refresh cron", opts: func() []Option {
			return []Option{
				WithJWKSRefreshSchedule(refreshEvery),
				WithRevocationStorage(&syncStorage{}), WithRevocationSyncSchedule(refreshEvery),
			}
		}},
		{name: "initial revocation sync fails", wantErr: true, opts: func() []Option {
			return []Option{WithRevocationStorage(&syncStorage{err: errors.New("sync failed")})}
		}},
		{name: "invalid refresh schedule", wantErr: true, opts: func() []Option {
			return []Option{WithJWKSRefreshSchedule("not a cron")}
		}},
		{name: "JWKS initialization fails", wantErr: true, opts: func() []Option {
			idp.jwksStatus.Store(http.StatusInternalServerError)
			return nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { idp.jwksStatus.Store(http.StatusOK) })
			baseline := runtime.NumGoroutine()

			for range 3 {
				p, err := idp.tryNewProvider(t, tc.opts()...)
				if tc.wantErr {
					require.Error(t, err)
					continue
				}
				require.NoError(t, err)
				p.Close()
			}

			testhelpers.WaitFor(t, 2*time.Second, func() bool {
				return runtime.NumGoroutine() <= baseline
			}, "providers leaked goroutines")
		})
	}
}

const revocationErrorsMetric = "test_auth_oidc_revocation_check_errors_total"

// The initial sync only counts as done when this provider completed a sync
// itself: a busy shared filter (peer rebuilding, or this rebuild superseded)
// is retried, and only running out of time applies the fail mode.
func TestInitialRevocationSync_RetriesWhileSharedFilterBusy(t *testing.T) {
	t.Parallel()

	inProgress := fmt.Errorf("acquire lease: %w", probfilter.ErrRebuildInProgress)
	superseded := fmt.Errorf("commit: %w", probfilter.ErrRebuildSuperseded)

	tests := []struct {
		name      string
		script    []error
		failOpen  bool
		wantErr   error
		wantSyncs int32
	}{
		{name: "peer publishes then own sync succeeds", script: []error{inProgress, inProgress, nil}, wantSyncs: 3},
		{name: "lease lost without successor then retry succeeds", script: []error{superseded, nil}, wantSyncs: 2},
		{name: "peer never finishes: fail-closed", script: []error{inProgress}, wantErr: probfilter.ErrRebuildInProgress},
		{name: "peer never finishes: fail-open starts", script: []error{inProgress}, failOpen: true},
		{name: "other errors are not retried", script: []error{errors.New("source down"), nil}, wantErr: ErrRevocationCheck, wantSyncs: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			collector := testhelpers.NewTestCollector()
			storage := &syncStorage{script: tc.script}
			opts := []Option{
				WithCollector(collector), WithRevocationStorage(storage),
				WithRevocationInitialSyncWait(time.Second),
			}
			if tc.failOpen {
				opts = append(opts, WithRevocationFailOpen())
			}

			_, err := idp.tryNewProvider(t, opts...)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, ErrRevocationCheck)
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantSyncs > 0 {
				require.Equal(t, tc.wantSyncs, storage.syncs.Load())
			}
			var wantCount float64
			if tc.wantErr != nil || tc.failOpen {
				wantCount = 1
			}
			require.Equal(t, wantCount, testhelpers.GetCounterValue(t, collector, revocationErrorsMetric))
		})
	}
}

// A scheduled sync that finds a peer rebuilding the shared filter is skipped
// silently; a superseded one is a counted failure.
func TestScheduledRevocationSync_SharedFilterOutcomes(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	collector := testhelpers.NewTestCollector()
	storage := &syncStorage{script: []error{
		nil, // construction
		fmt.Errorf("acquire lease: %w", probfilter.ErrRebuildInProgress),
		fmt.Errorf("commit: %w", probfilter.ErrRebuildSuperseded),
	}}
	p := idp.newProvider(t, WithCollector(collector), WithRevocationStorage(storage))

	require.NoError(t, p.scheduledRevocationSync(t.Context()))
	require.Zero(t, testhelpers.GetCounterValue(t, collector, revocationErrorsMetric))

	require.ErrorIs(t, p.scheduledRevocationSync(t.Context()), probfilter.ErrRebuildSuperseded)
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, collector, revocationErrorsMetric))
}
