// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal

import "context"

// contextKey is the context key under which [NewContext] and
// [NewContextFrom] store a Principal.
type contextKey struct{}

// NewContext returns a copy of ctx carrying p as the request's verified
// principal. Transport authentication adapters call it after a successful
// authentication whose result is a Principal, so a handler served over HTTP
// and gRPC reads the caller the same way through [FromContext].
func NewContext(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

// NewContextFrom returns a copy of ctx carrying v as the request's verified
// principal when v is a Principal or a non-nil *Principal, and reports
// whether it did; any other v leaves ctx unchanged. A Principal already held
// in v is stored as is, without boxing it again, so a caller that has an
// authenticator's result as an interface value pays only for the context
// node; a *Principal is copied, as by [NewContext].
func NewContextFrom(ctx context.Context, v any) (context.Context, bool) {
	switch p := v.(type) {
	case Principal:
		return context.WithValue(ctx, contextKey{}, v), true
	case *Principal:
		if p != nil {
			return NewContext(ctx, *p), true
		}
	}
	return ctx, false
}

// FromContext returns the Principal stored by [NewContext] or
// [NewContextFrom]. The bool is false when ctx carries none, for example when
// the request was not authenticated or its authenticator returned an
// application-defined identity type.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}
