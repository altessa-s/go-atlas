// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestRegistrationChangesDuringCheck(t *testing.T) {
	t.Parallel()
	for _, api := range []string{"overall", "cached overall", "service", "cached service", "list"} {
		for _, replace := range []bool{false, true} {
			name := "remove"
			if replace {
				name = "replace"
			}
			t.Run(api+"/"+name, func(t *testing.T) {
				t.Parallel()
				c := health.New(health.WithCheckTimeout(10*time.Second), health.WithStatusCacheTTL(time.Minute))
				t.Cleanup(c.Close)
				entered, release := make(chan struct{}), make(chan struct{})
				checked := health.StatusNotServing
				if replace {
					checked = health.StatusServing
				}
				c.RegisterService("db", health.Func(func(ctx context.Context) health.ServingStatus {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
					}
					return checked
				}))
				done := make(chan health.ServingStatus, 1)
				go func() {
					switch api {
					case "overall":
						done <- c.CheckHealth(t.Context())
					case "cached overall":
						done <- c.CheckStatus(t.Context(), "")
					case "service":
						done <- c.CheckServiceHealth(t.Context(), "db")
					case "cached service":
						done <- c.CheckStatus(t.Context(), "db")
					case "list":
						statuses, err := c.ListStatuses(t.Context())
						if err != nil {
							done <- health.StatusUnknown
							return
						}
						done <- statuses["db"]
					}
				}()
				<-entered
				unregistered := make(chan struct{})
				go func() {
					c.UnregisterService("db")
					close(unregistered)
				}()
				require.Eventually(t, func() bool {
					return !slices.Contains(slices.Collect(c.ListServices()), "db")
				}, time.Second, time.Millisecond)
				var replacementCalls atomic.Int32
				if replace {
					c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus {
						replacementCalls.Add(1)
						return health.StatusNotServing
					}))
				}
				close(release)
				<-unregistered
				want := health.StatusServing
				if replace {
					want = health.StatusNotServing
				} else if api == "service" || api == "cached service" {
					want = health.StatusServiceUnknown
				}
				if api == "list" && !replace {
					want = health.StatusUnknown
				}
				require.Equal(t, want, <-done)
				require.Equal(t, 0, c.GetMetrics()["cached_statuses"], "a changed registration must not populate the cache")
				if replace {
					require.Equal(t, health.StatusNotServing, c.CheckStatus(t.Context(), ""))
					require.Positive(t, replacementCalls.Load(), "the replacement must be checked on the next call")
				}
			})
		}
	}
}

func TestUnregisterWaitsForActiveCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		scoped bool
	}{{"named", false}, {"scoped", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				c := health.New()
				entered, release := make(chan struct{}), make(chan struct{})
				resume := sync.OnceFunc(func() { close(release) })
				defer resume()
				calls := 0
				checker := health.Func(func(context.Context) health.ServingStatus {
					calls++
					if calls == 1 {
						close(entered)
						<-release
					}
					return health.StatusServing
				})
				var unregister func()
				if tc.scoped {
					_, unregister = c.RegisterScopedService("db", checker)
				} else {
					c.RegisterService("db", checker)
					unregister = func() { c.UnregisterService("db") }
				}
				go c.CheckServiceHealth(t.Context(), "db")
				<-entered
				completed := make(chan bool, 2)
				for range 2 {
					go func() {
						unregister()
						completed <- true
					}()
				}
				synctest.Wait()
				require.Empty(t, completed)
				require.Equal(t, health.StatusServiceUnknown, c.CheckServiceHealth(t.Context(), "db"))
				resume()
				<-completed
				<-completed
				require.Equal(t, health.StatusServiceUnknown, c.CheckServiceHealth(t.Context(), "db"))
				require.Equal(t, 1, calls)
			})
		})
	}
}

func TestScopedUnregisterLeavesReplacement(t *testing.T) {
	t.Parallel()
	c := health.New()
	_, unregister := c.RegisterScopedService("db", health.Func(func(context.Context) health.ServingStatus {
		return health.StatusNotServing
	}))
	c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus {
		return health.StatusServing
	}))
	unregister()
	require.Equal(t, health.StatusServing, c.CheckServiceHealth(t.Context(), "db"))
}

func TestRegistrationInvalidatesCachedStatuses(t *testing.T) {
	t.Parallel()
	for _, remove := range []bool{false, true} {
		name := "replace"
		if remove {
			name = "remove"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := health.New(health.WithStatusCacheTTL(time.Minute))
			t.Cleanup(c.Close)
			c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus { return health.StatusServing }))
			require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), "db"))
			require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), ""))
			if remove {
				c.UnregisterService("db")
			} else {
				c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus { return health.StatusNotServing }))
			}
			require.Equal(t, 0, c.GetMetrics()["cached_statuses"])
			if !remove {
				require.Equal(t, health.StatusNotServing, c.CheckStatus(t.Context(), "db"))
				require.Equal(t, health.StatusNotServing, c.CheckStatus(t.Context(), ""))
			}
		})
	}
}
