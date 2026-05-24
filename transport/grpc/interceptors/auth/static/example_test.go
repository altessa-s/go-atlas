// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"fmt"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"

	"google.golang.org/grpc"
)

// UserInfo represents the data stored alongside each token.
type UserInfo struct {
	ID    string
	Roles []string
}

func ExampleAuthFunc() {
	store := static.NewInMemoryStore(
		static.WithInitialTokens(map[string]any{
			"valid_token": UserInfo{ID: "user1", Roles: []string{"admin"}},
		}),
	)

	interceptor := auth.ServerInterceptor(
		auth.WithAuthFn(static.AuthFunc(store)),
	)

	_ = grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
		grpc.StreamInterceptor(interceptor.ServerStreamInterceptor()),
	)

	fmt.Println("server configured with static token authentication")
	// Output: server configured with static token authentication
}

func ExampleAuthFunc_apiKey() {
	store := static.NewInMemoryStore(
		static.WithInitialTokens(map[string]any{
			"api_key_123": UserInfo{ID: "service1"},
		}),
	)

	apiKeyExtractor := auth.ExtractTokenFromHeader("x-api-key", func(v string) (string, error) {
		return v, nil
	})

	interceptor := auth.ServerInterceptor(
		auth.WithTokenExtractor(apiKeyExtractor),
		auth.WithAuthFn(static.AuthFunc(store)),
	)

	_ = grpc.NewServer(grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()))

	fmt.Println("server configured with API key authentication")
	// Output: server configured with API key authentication
}
