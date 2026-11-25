// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/internal/grpcstatus"

	"google.golang.org/grpc/status"
)

// ClientInterceptor returns a client interceptor that converts gRPC status errors
// into application errors using the configured [StatusConverter] entries.
func ClientInterceptor(opt ...Option) interceptors.ClientInterceptor {
	opts := newOptions(opt...)
	if len(opts.statusConverter) == 0 {
		return grpcstatus.New(opts.logger, nil)
	}
	return grpcstatus.New(opts.logger, func(ctx context.Context, st *status.Status) (error, bool) {
		for _, converter := range opts.statusConverter {
			if converter.Matcher(ctx, st) {
				return converter.Convert(ctx, st), true
			}
		}
		return nil, false
	})
}
