// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/static"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthFunc returns an [auth.AuthFunc] that validates the credential token
// against store. Errors from store are mapped to gRPC status codes:
//
//   - [static.ErrTokenInvalid] / [static.ErrTokenEmpty] → codes.Unauthenticated
//   - [static.ErrRateLimited]                          → codes.ResourceExhausted
//   - any other error                                  → codes.Internal
//
// Pass [WithAudit] to record each authentication decision through an
// [auth.DecisionRecorder].
//
// Panics if store is nil — an unauthenticated server is not a usable default.
func AuthFunc(store static.TokenStore, opts ...AuthOption) auth.AuthFunc {
	if store == nil {
		panic("transport/grpc/.../auth/static: AuthFunc requires a non-nil TokenStore")
	}
	var cfg authConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(ctx context.Context, request auth.Request) (any, error) {
		creds, ok := request.TokenCredentials()
		if !ok {
			return nil, cfg.finalize(ctx, nil, false, "missing_token",
				status.Error(codes.Unauthenticated, "missing token credentials"))
		}
		data, err := store.Validate(ctx, creds.Token.Expose())
		if err != nil {
			return nil, cfg.finalize(ctx, nil, false, auditReason(err), toGRPCStatus(err))
		}
		if ferr := cfg.finalize(ctx, data, true, "", nil); ferr != nil {
			return nil, ferr
		}
		return data, nil
	}
}

// AuthOption configures [AuthFunc].
type AuthOption func(*authConfig)

type authConfig struct {
	recorder  auth.DecisionRecorder
	subjectOf func(any) string
}

// WithAudit records each authentication decision through rec, with action
// "authenticate". subjectOf extracts the principal identity from the value the
// store returned on success; pass nil to leave the subject empty. When rec is
// configured with decision.FailureRequired and recording an otherwise-successful
// authentication fails, the call is failed with codes.Internal so nothing
// proceeds unrecorded.
func WithAudit(rec auth.DecisionRecorder, subjectOf func(any) string) AuthOption {
	return func(c *authConfig) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c authConfig) finalize(ctx context.Context, data any, allowed bool, reason string, authErr error) error {
	if c.recorder == nil {
		return authErr
	}
	subject := ""
	if allowed && c.subjectOf != nil {
		subject = c.subjectOf(data)
	}
	recErr := c.recorder.Record(ctx, decision.Decision{
		Time:       time.Now().UTC(),
		Allowed:    allowed,
		Subject:    subject,
		Action:     "authenticate",
		Reason:     reason,
		Attributes: map[string]string{"transport": "grpc"},
	})
	if recErr != nil && authErr == nil {
		return status.Error(codes.Internal, "authentication audit failed")
	}
	return authErr
}

// auditReason maps a store error to a stable audit reason token.
func auditReason(err error) string {
	switch {
	case errors.Is(err, static.ErrTokenInvalid):
		return "invalid_token"
	case errors.Is(err, static.ErrTokenEmpty):
		return "empty_token"
	case errors.Is(err, static.ErrRateLimited):
		return "rate_limited"
	default:
		return "error"
	}
}

func toGRPCStatus(err error) error {
	switch {
	case errors.Is(err, static.ErrTokenInvalid), errors.Is(err, static.ErrTokenEmpty):
		return status.Error(codes.Unauthenticated, "invalid token")
	case errors.Is(err, static.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "too many authentication attempts")
	default:
		// Unknown store errors (driver messages, connection strings) must not reach
		// the client; the failure category is still recorded via the audit reason.
		return status.Error(codes.Internal, "Internal Server Error")
	}
}
