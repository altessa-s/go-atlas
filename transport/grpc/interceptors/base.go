// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/internal/base"
)

// BaseInterceptor provides common functionality for gRPC interceptors.
// It handles endpoint filtering (ignore patterns), logging, and name identification.
//
// Embed this struct in your interceptor implementation to get common functionality:
//
//	type myInterceptor struct {
//	    interceptors.BaseInterceptor
//	    // ... your fields
//	}
//
// For interceptors without ignore patterns:
//
//	func NewMyInterceptor(logger *slog.Logger) *myInterceptor {
//	    return &myInterceptor{
//	        BaseInterceptor: interceptors.NewBaseInterceptor("myinterceptor", logger),
//	    }
//	}
//
// For interceptors with ignore patterns:
//
//	func NewMyInterceptor(ignoreMethods []string, ignorePatterns []*regexp.Regexp, logger *slog.Logger) *myInterceptor {
//	    return &myInterceptor{
//	        BaseInterceptor: interceptors.NewBaseInterceptorWithFilter("myinterceptor", ignoreMethods, ignorePatterns, logger),
//	    }
//	}
type BaseInterceptor struct {
	base.Base
}

// NewBaseInterceptor creates a new BaseInterceptor with the given name and logger.
// This is the basic constructor for interceptors that don't need method filtering.
//
// Parameters:
//   - name: The interceptor name used for identification and ordering
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseInterceptor(name string, logger *slog.Logger) BaseInterceptor {
	return BaseInterceptor{Base: base.New(name, "interceptor", "method", logger)}
}

// NewBaseInterceptorWithFilter creates a new BaseInterceptor with method filtering support.
// Use this constructor for interceptors that need to skip certain methods.
//
// Parameters:
//   - name: The interceptor name used for identification and ordering
//   - ignoreMethods: List of method names to skip (e.g., "/grpc.health.v1.Health/Check")
//   - ignorePatterns: Regex patterns for methods to skip
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseInterceptorWithFilter(name string, ignoreMethods []string, ignorePatterns []*regexp.Regexp, logger *slog.Logger) BaseInterceptor {
	return BaseInterceptor{Base: base.NewWithFilter(name, "interceptor", "method", ignoreMethods, ignorePatterns, logger)}
}

// InternMethod returns an interned version of the method name for memory efficiency.
func (b *BaseInterceptor) InternMethod(method string) string {
	return b.InternEndpoint(method)
}

// ShouldIgnoreFromContext checks if the current call should be ignored based on CallMetadata.
// Returns true if the method should be skipped, along with the CallMetadata.
func (b *BaseInterceptor) ShouldIgnoreFromContext(ctx context.Context) (*metadata.CallMetadata, bool) {
	meta, ok := metadata.FromContext(ctx)
	if !ok {
		return nil, false
	}
	return meta, b.ShouldIgnore(meta.FullyMethodName)
}
