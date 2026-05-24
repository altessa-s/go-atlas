// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"
)

func generateTokens(count int) map[string]any {
	tokens := make(map[string]any, count)
	for i := range count {
		tokens["token_"+strconv.Itoa(i)] = userInfo{
			ID:   "user_" + strconv.Itoa(i),
			Role: "role_" + strconv.Itoa(i%3),
		}
	}
	return tokens
}

func BenchmarkAuthFunc_Hit(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	fn := static.AuthFunc(store)
	ctx := b.Context()
	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "token_500"},
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = fn(ctx, req)
	}
}

func BenchmarkAuthFunc_Miss(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	fn := static.AuthFunc(store)
	ctx := b.Context()
	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "no_such_token"},
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = fn(ctx, req)
	}
}

func BenchmarkAuthFunc_Parallel(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	fn := static.AuthFunc(store)
	ctx := b.Context()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := auth.Request{
				Base:    auth.Base{AuthMethod: auth.MethodToken},
				Payload: &auth.TokenCredentials{Token: "token_" + strconv.Itoa(i%1000)},
			}
			_, _ = fn(ctx, req)
			i++
		}
	})
}
