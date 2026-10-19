// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/core/encoding/etag"
	"github.com/altessa-s/go-atlas/data/keyset"
	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// handlerViolation is the status a handler used to build itself for an
// invalid request field and return as is: InvalidArgument "<path>: <reason>"
// with one altessa BadRequest violation (path and message, no code). It is
// frozen here so the tests below pin that a domain FieldError converted by
// the canonical rules reaches the client byte-identical to it.
func handlerViolation(path, reason string) error {
	st := status.New(codes.InvalidArgument, path+": "+reason)
	st, err := st.WithDetails(&badrequestv1.BadRequest{
		FieldViolations: []*badrequestv1.FieldViolation{{FieldPath: new(path), Message: new(reason)}},
	})
	if err != nil {
		panic(err)
	}
	return st.Err()
}

// requireSameStatus asserts that want and got reach the client as equal
// status protos (code, message and every detail).
func requireSameStatus(t *testing.T, want, got error) {
	t.Helper()
	ws, ok := status.FromError(want)
	require.True(t, ok, "want = %v", want)
	gs, ok := status.FromError(got)
	require.True(t, ok, "got = %v", got)
	require.True(t, proto.Equal(ws.Proto(), gs.Proto()), "want %v\ngot  %v", ws.Proto(), gs.Proto())
}

// TestRequestFieldErrorsMatchHandlerViolations covers the request-field
// failures a list or update handler reports (page token, page size, update
// mask, etag): returned as a core/errors.FieldError that wraps the
// underlying error, they reach the client exactly like the violation status
// a handler built itself, with and without a field path mapper.
func TestRequestFieldErrorsMatchHandlerViolations(t *testing.T) {
	t.Parallel()

	wrap := func(err error) error { return fmt.Errorf("list: %w", err) }
	pageToken := func(field string, err error) error {
		return &coreerrs.FieldError{Field: field, Reason: keyset.Classify(err).String(), Err: err}
	}
	_, wildcardErr := fieldmask.FromPaths("*", "name").SelectsAll()
	_, missingEtag := etag.ParseVersion("")
	_, malformedEtag := etag.ParseVersion(`"3"`)

	tests := []struct {
		name         string
		err          error
		path, reason string
	}{
		{"page token malformed", pageToken("pageToken", wrap(keyset.ErrInvalidToken)), "pageToken", "malformed"},
		{"page token rebound", pageToken("pageToken", wrap(keyset.ErrFilterChanged)), "pageToken", "malformed"},
		{"page token without codec", pageToken("page_token", wrap(keyset.ErrCodecRequired)), "page_token", "malformed"},
		{"page token expired", pageToken("pageToken", wrap(keyset.ErrExpiredToken)), "pageToken", "expired"},
		{"negative page size", coreerrs.NewFieldError("pageSize", "must not be negative"), "pageSize", "must not be negative"},
		{
			"wildcard not alone", &coreerrs.FieldError{Field: "updateMask", Reason: `"*" must be the only path`, Err: wildcardErr},
			"updateMask", `"*" must be the only path`,
		},
		{"etag missing", &coreerrs.FieldError{Field: "bot.etag", Reason: "required", Err: missingEtag}, "bot.etag", "required"},
		{"etag malformed", &coreerrs.FieldError{Field: "etag", Reason: "invalid value", Err: malformedEtag}, "etag", "invalid value"},
	}
	// A path mapper spells the field of a FieldError (a status the handler
	// built itself passes through as is), so under CamelPath the expected
	// violation names the camel-cased path.
	modes := map[string]struct {
		opts []Option
		path PathMapper
	}{
		"as written":  {[]Option{WithCanonicalRules(), WithFinalizer(DefaultFinalizerWithDomain("svc"))}, func(p string) string { return p }},
		"camel paths": {[]Option{WithCanonicalRules(), WithFieldPathMapper(CamelPath), WithFinalizer(DefaultFinalizerWithDomain("svc"))}, CamelPath},
	}
	for mode, m := range modes {
		for _, tc := range tests {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				got := callWith(t, tc.err, m.opts...)
				path := m.path(tc.path)
				requireSameStatus(t, callWith(t, handlerViolation(path, tc.reason), m.opts...), got)
				requireBadRequest(t, got, path, tc.reason, "svc")
			})
		}
	}

	t.Run("snake update mask field mapped to camel case", func(t *testing.T) {
		t.Parallel()
		opts := modes["camel paths"].opts
		err := &coreerrs.FieldError{Field: fieldmask.DefaultUpdateMaskFieldName, Reason: `"*" must be the only path`, Err: wildcardErr}
		requireSameStatus(t, callWith(t, handlerViolation("updateMask", `"*" must be the only path`), opts...), callWith(t, err, opts...))
	})

	t.Run("the wrapped cause stays matchable", func(t *testing.T) {
		t.Parallel()
		err := pageToken("pageToken", wrap(keyset.ErrExpiredToken))
		require.ErrorIs(t, err, keyset.ErrExpiredToken)
		require.ErrorIs(t, &coreerrs.FieldError{Field: "etag", Reason: "required", Err: missingEtag}, etag.ErrMissing)
		require.True(t, errors.Is(wildcardErr, fieldmask.ErrWildcardNotAlone))
	})
}

// TestEtagMismatchIsAConflict pins that etag.CheckVersion's mismatch is
// reported by the canonical conflict rule.
func TestEtagMismatchIsAConflict(t *testing.T) {
	t.Parallel()

	err := etag.CheckVersion(`W/"2"`, 3)
	got := callWith(t, fmt.Errorf("update bot b1: %w", err), WithCanonicalRules(), WithFinalizer(DefaultFinalizerWithDomain("svc")))
	st, ok := status.FromError(got)
	require.True(t, ok)
	require.Equal(t, codes.Aborted, st.Code())
	require.Equal(t, MessageConflict, st.Message())
	require.Len(t, st.Details(), 1)
	ei, ok := st.Details()[0].(*errdetails.ErrorInfo)
	require.True(t, ok)
	require.Equal(t, ReasonVersionConflict, ei.GetReason())
	require.Equal(t, "svc", ei.GetDomain())
}
