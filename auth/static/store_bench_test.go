// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/auth/static"
)

func generateTokens(count int) map[string]any {
	tokens := make(map[string]any, count)
	for i := range count {
		tokens["token_"+strconv.Itoa(i)] = userInfo{
			ID:    "user_" + strconv.Itoa(i),
			Role:  "role_" + strconv.Itoa(i%3),
			Email: "user" + strconv.Itoa(i) + "@example.com",
		}
	}
	return tokens
}

func BenchmarkInMemoryStore_Validate_Hit(b *testing.B) {
	s := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	ctx := b.Context()
	token := "token_500"
	b.ResetTimer()
	for b.Loop() {
		_, _ = s.Validate(ctx, token)
	}
}

func BenchmarkInMemoryStore_Validate_Miss(b *testing.B) {
	s := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	ctx := b.Context()
	token := "no_such_token"
	b.ResetTimer()
	for b.Loop() {
		_, _ = s.Validate(ctx, token)
	}
}

func BenchmarkInMemoryStore_AddToken(b *testing.B) {
	s := static.NewInMemoryStore()
	info := userInfo{ID: "user", Role: "admin"}
	i := 0
	b.ResetTimer()
	for b.Loop() {
		s.AddToken("token_"+strconv.Itoa(i), info)
		i++
	}
}

func BenchmarkInMemoryStore_Validate_Parallel(b *testing.B) {
	s := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		ctx := context.Background()
		for pb.Next() {
			_, _ = s.Validate(ctx, "token_"+strconv.Itoa(i%1000))
			i++
		}
	})
}
