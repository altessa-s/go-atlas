// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestRedactPayload verifies the payload logging hook: without a redactor the raw
// proto is logged, and with one the redactor's output replaces it — the only way
// to keep sensitive fields out of request/response payload logs.
func TestRedactPayload(t *testing.T) {
	t.Parallel()

	msg := &emptypb.Empty{}

	t.Run("no redactor logs raw proto", func(t *testing.T) {
		t.Parallel()
		ri := &requestInterceptor{interceptor: &interceptor{opts: newOptions()}}
		got := ri.redactPayload(msg)
		require.NotNil(t, got)
		require.NotEqual(t, "[REDACTED]", got)
	})

	t.Run("redactor output replaces payload", func(t *testing.T) {
		t.Parallel()
		ri := &requestInterceptor{interceptor: &interceptor{
			opts: newOptions(WithPayloadRedactor(func(proto.Message) any { return "[REDACTED]" })),
		}}
		require.Equal(t, "[REDACTED]", ri.redactPayload(msg))
	})
}
