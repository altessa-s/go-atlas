// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"

	redacttestv1 "github.com/altessa-s/go-atlas/proto/gen/redacttest/v1"
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

	t.Run("debug redact clears marked fields", func(t *testing.T) {
		t.Parallel()
		in := &redacttestv1.Provider{Name: "p", ApiKey: proto.String("secret")}
		ri := &requestInterceptor{interceptor: &interceptor{opts: newOptions(WithDebugRedact())}}
		got, ok := ri.redactPayload(in).(*redacttestv1.Provider)
		require.True(t, ok)
		require.Equal(t, "p", got.GetName())
		require.Nil(t, got.ApiKey)
		require.Equal(t, "secret", in.GetApiKey())
	})

	t.Run("debug redact runs before the payload redactor", func(t *testing.T) {
		t.Parallel()
		var seen proto.Message
		ri := &requestInterceptor{interceptor: &interceptor{opts: newOptions(
			WithDebugRedact(),
			WithPayloadRedactor(func(m proto.Message) any { seen = m; return "custom" }),
		)}}
		require.Equal(t, "custom", ri.redactPayload(&redacttestv1.Provider{Name: "p", ApiKey: proto.String("secret")}))
		got, ok := seen.(*redacttestv1.Provider)
		require.True(t, ok)
		require.Nil(t, got.ApiKey, "the payload redactor receives the debug-redacted copy")
	})

	t.Run("debug redact off by default", func(t *testing.T) {
		t.Parallel()
		in := &redacttestv1.Provider{ApiKey: proto.String("secret")}
		ri := &requestInterceptor{interceptor: &interceptor{opts: newOptions()}}
		got, ok := ri.redactPayload(in).(*redacttestv1.Provider)
		require.True(t, ok)
		require.Equal(t, "secret", got.GetApiKey())
	})
}

// TestRedactAnyBoundedWork packs a secret under 2000 Any layers: the unpacking stops at maxAnyDepth (the deeper payload becomes the
// empty placeholder), the decoded bytes stay within the shared budget, and the input is unchanged.
func TestRedactAnyBoundedWork(t *testing.T) {
	t.Parallel()

	var packed proto.Message = &redacttestv1.Provider{Name: "p", ApiKey: proto.String("secret")}
	for range 2000 {
		a, err := anypb.New(packed)
		require.NoError(t, err)
		packed = a
	}
	in := packed.(*anypb.Any) //nolint:forcetypeassert // built above
	before, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	require.NoError(t, err)

	out := proto.Clone(in)
	w := new(redactWalk)
	w.clear(out.ProtoReflect())
	require.LessOrEqual(t, w.decoded, maxAnyDecodedBytes)
	require.Zero(t, w.anyDepth)

	// Within maxAnyDepth layers (sooner when the decode budget runs out first) the payload becomes the placeholder.
	layer := out.(*anypb.Any) //nolint:forcetypeassert // clone of an Any
	layers := 1
	for layer.GetTypeUrl() != emptyAnyURL {
		next := new(anypb.Any)
		require.NoError(t, layer.UnmarshalTo(next))
		layer = next
		layers++
		require.LessOrEqual(t, layers, maxAnyDepth+1)
	}
	require.Empty(t, layer.GetValue())

	after, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, before, after, "the input must not change")

}

// TestRedactAnyAllocsDoNotGrowWithNesting: 2000 Any layers allocate no more than 100 do. It is not parallel: testing.AllocsPerRun
// refuses to run in a parallel test.
func TestRedactAnyAllocsDoNotGrowWithNesting(t *testing.T) { //nolint:paralleltest // AllocsPerRun forbids t.Parallel
	allocs := func(depth int) float64 {
		var m proto.Message = &redacttestv1.Provider{ApiKey: proto.String("s")}
		for range depth {
			a, err := anypb.New(m)
			require.NoError(t, err)
			m = a
		}
		return testing.AllocsPerRun(5, func() { new(redactWalk).clear(proto.Clone(m).ProtoReflect()) })
	}
	shallow, deep := allocs(100), allocs(2000)
	require.LessOrEqual(t, deep, shallow+50, "allocations must not grow with the nesting (100 layers: %v, 2000: %v)", shallow, deep)
}
