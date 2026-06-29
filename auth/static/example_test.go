// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/auth/static"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// User represents the data stored alongside each token.
type User struct {
	ID    string
	Roles []string
}

func ExampleNewInMemoryStore() {
	store := static.NewInMemoryStore(
		static.WithInitialTokens(map[string]any{
			"sk_live_abcd1234": User{ID: "user1", Roles: []string{"admin"}},
		}),
	)

	data, err := store.Validate(context.Background(), "sk_live_abcd1234")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(data.(User).ID)
	// Output: user1
}

func ExampleInMemoryStore_AddToken() {
	store := static.NewInMemoryStore()
	store.AddToken("dynamic_token", User{ID: "user2"})

	data, err := store.Validate(context.Background(), "dynamic_token")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(data.(User).ID)
	// Output: user2
}

func ExampleNewInMemoryStore_withMetrics() {
	m := static.NewMetrics(metrics.Noop(), "auth_api")
	store := static.NewInMemoryStore(
		static.WithInitialTokens(map[string]any{"t": User{ID: "u"}}),
		static.WithMetrics(m),
	)

	_, _ = store.Validate(context.Background(), "t")
	fmt.Println("validated")
	// Output: validated
}

// quotaLimiter is a tiny demo limiter — production code should plug a real
// implementation, e.g. one backed by data/limiters/tokenbucket.
//
// It demonstrates the failure-only contract: Allow is a pure check that
// reports whether the per-key budget is exhausted, and RecordFailure is
// the only path that debits it.
type quotaLimiter struct{ remainingFailures int }

func (l *quotaLimiter) Allow(context.Context, string) bool {
	return l.remainingFailures > 0
}

func (l *quotaLimiter) RecordFailure(context.Context, string) {
	if l.remainingFailures > 0 {
		l.remainingFailures--
	}
}

func ExampleNewRateLimitedStore() {
	inner := static.NewInMemoryStore(
		static.WithInitialTokens(map[string]any{"good": User{ID: "u1"}}),
	)
	// Budget: one failure allowed before Allow starts denying. Successful
	// validations never touch the budget.
	store := static.NewRateLimitedStore(inner, &quotaLimiter{remainingFailures: 1},
		func(context.Context) string { return "client-1" })

	// First bad attempt: passes Allow (budget=1), validation fails,
	// RecordFailure drops the budget to zero.
	_, err := store.Validate(context.Background(), "bad")
	fmt.Println(errors.Is(err, static.ErrTokenInvalid))

	// Second bad attempt: Allow now denies — budget exhausted.
	_, err = store.Validate(context.Background(), "bad")
	fmt.Println(errors.Is(err, static.ErrRateLimited))

	// Successful validation goes through even after a prior failure,
	// since success does not consult the (already-deny) budget — wait,
	// it does: Allow still gates EVERY request. The valid call is also
	// rate-limited until the budget refills. This is the intended
	// failure-only-decay contract; pick a self-decaying limiter in
	// production so legitimate traffic resumes after a quiet window.
	_, err = store.Validate(context.Background(), "good")
	fmt.Println(errors.Is(err, static.ErrRateLimited))
	// Output:
	// true
	// true
	// true
}
