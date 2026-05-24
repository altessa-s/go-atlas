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
type quotaLimiter struct{ remaining int }

func (l *quotaLimiter) Allow(context.Context, string) bool {
	if l.remaining <= 0 {
		return false
	}
	l.remaining--
	return true
}

func (l *quotaLimiter) Reset(string) { l.remaining = 5 }

func ExampleNewRateLimitedStore() {
	inner := static.NewInMemoryStore(
		static.WithInitialTokens(map[string]any{"good": User{ID: "u1"}}),
	)
	store := static.NewRateLimitedStore(inner, &quotaLimiter{remaining: 1},
		func(context.Context) string { return "client-1" })

	_, err := store.Validate(context.Background(), "bad")
	fmt.Println(errors.Is(err, static.ErrInvalidToken))

	// Second attempt is rejected by the limiter before reaching the store.
	_, err = store.Validate(context.Background(), "good")
	fmt.Println(errors.Is(err, static.ErrRateLimited))
	// Output:
	// true
	// true
}
