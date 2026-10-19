// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/altessa-s/go-atlas/proto/gen/redacttest/v1"
)

const secret = "sk-ant-api03-very-secret"

// logged renders a redacted payload the way it reaches a log (JSON and fmt) and fails when the encoding itself would already hide
// the secret, so a test cannot pass vacuously.
func logged(t *testing.T, in, out any) string {
	t.Helper()
	inMsg, ok := in.(proto.Message)
	require.True(t, ok)
	raw, err := protojson.Marshal(inMsg)
	require.NoError(t, err)
	require.Contains(t, string(raw), secret, "the JSON encoding already hides the secret; the test proves nothing")

	outMsg, ok := out.(proto.Message)
	require.True(t, ok, "DebugRedact returned %T, want proto.Message", out)
	raw, err = protojson.Marshal(outMsg)
	require.NoError(t, err)
	return string(raw) + fmt.Sprint(outMsg)
}

func TestDebugRedactClearsKey(t *testing.T) {
	t.Parallel()

	in := &pb.Provider{Name: "p", ApiKey: proto.String(secret), Status: &pb.Status{State: "ok"}}
	out := logger.DebugRedact(in)

	text := logged(t, in, out)
	require.NotContains(t, text, secret)
	require.Contains(t, text, `"name":"p"`)
	require.Contains(t, text, `"state":"ok"`)
	require.Equal(t, secret, in.GetApiKey(), "the message itself must not change")
}

func TestDebugRedactClearsNestedKeys(t *testing.T) {
	t.Parallel()

	in := &pb.ListProvidersResponse{
		Providers: []*pb.Provider{{Name: "a", ApiKey: proto.String(secret)}, {Name: "b", ApiKey: proto.String(secret)}},
		ByName:    map[string]*pb.Provider{"c": {Name: "c", ApiKey: proto.String(secret)}},
		Root:      &pb.Node{Label: "l0", Token: secret, Child: &pb.Node{Label: "l1", Token: secret, Child: &pb.Node{Label: "l2", Token: secret}}},
	}
	out := logger.DebugRedact(in)

	text := logged(t, in, out)
	require.NotContains(t, text, secret)
	for _, kept := range []string{`"name":"a"`, `"name":"b"`, `"name":"c"`, `"label":"l2"`} {
		require.Contains(t, text, kept)
	}
	for _, p := range in.GetProviders() {
		require.Equal(t, secret, p.GetApiKey(), "the message itself must not change")
	}
	require.Equal(t, secret, in.GetByName()["c"].GetApiKey())
	require.Equal(t, secret, in.GetRoot().GetChild().GetChild().GetToken())
}

// TestDebugRedactKeepsPlainMessages: a message type that cannot carry a redacted field is logged as it is, without an allocating
// copy — also a recursive one.
func TestDebugRedactKeepsPlainMessages(t *testing.T) {
	t.Parallel()

	for _, m := range []proto.Message{&emptypb.Empty{}, &pb.Status{State: "x"}, &pb.Tree{Value: "v", Left: &pb.Tree{Value: "w"}}} {
		require.Same(t, m, logger.DebugRedact(m))
	}
	require.Nil(t, logger.DebugRedact(nil))
}

// TestDebugRedactUnsetSecret: a message whose type can carry a secret but does not is still copied, and its content is kept.
func TestDebugRedactUnsetSecret(t *testing.T) {
	t.Parallel()

	in := &pb.Provider{Name: "p"}
	out, ok := logger.DebugRedact(in).(proto.Message)
	require.True(t, ok)
	require.NotSame(t, in, out)
	require.True(t, proto.Equal(in, out))
}

func TestDebugRedactConcurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 50 {
				out, _ := logger.DebugRedact(&pb.Provider{Name: "p", ApiKey: proto.String(secret)}).(proto.Message)
				if strings.Contains(fmt.Sprint(out), secret) {
					t.Error("secret logged")
					return
				}
			}
		})
	}
	wg.Wait()
}

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	require.NoError(t, err)
	return a
}

func unpackProvider(t *testing.T, a *anypb.Any) *pb.Provider {
	t.Helper()
	var p pb.Provider
	require.NoError(t, a.UnmarshalTo(&p))
	return &p
}

// TestDebugRedactAny: a message packed in google.protobuf.Any — alone or inside a field, a list or a map value — is unpacked,
// redacted and repacked in the copy; the original keeps its secret.
func TestDebugRedactAny(t *testing.T) {
	t.Parallel()

	t.Run("direct", func(t *testing.T) {
		t.Parallel()
		in := mustAny(t, &pb.Provider{Name: "p", ApiKey: proto.String(secret)})
		out, ok := logger.DebugRedact(in).(*anypb.Any)
		require.True(t, ok)
		got := unpackProvider(t, out)
		require.Equal(t, "p", got.GetName())
		require.Nil(t, got.ApiKey)
		require.NotContains(t, string(out.GetValue()), secret)
		require.Equal(t, secret, unpackProvider(t, in).GetApiKey(), "the message itself must not change")
	})

	t.Run("nested in a field, a list and a map", func(t *testing.T) {
		t.Parallel()
		in := &pb.Envelope{
			Payload: mustAny(t, &pb.Provider{Name: "a", ApiKey: proto.String(secret)}),
			Items:   []*anypb.Any{mustAny(t, &pb.Node{Label: "n", Token: secret}), mustAny(t, &pb.Provider{Name: "b", ApiKey: proto.String(secret)})},
			ByName:  map[string]*anypb.Any{"c": mustAny(t, &pb.Envelope{Payload: mustAny(t, &pb.Provider{Name: "c", ApiKey: proto.String(secret)})})},
		}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		raw, err := protojson.Marshal(out)
		require.NoError(t, err)
		require.NotContains(t, string(raw), secret)
		require.Contains(t, string(raw), `"name":"c"`)
		for _, a := range []*anypb.Any{out.GetPayload(), out.GetItems()[0], out.GetItems()[1], out.GetByName()["c"]} {
			require.NotContains(t, string(a.GetValue()), secret)
		}
		require.Equal(t, secret, unpackProvider(t, in.GetPayload()).GetApiKey(), "the message itself must not change")
	})

	t.Run("payload without secrets is kept as is", func(t *testing.T) {
		t.Parallel()
		in := &pb.Envelope{Payload: mustAny(t, &pb.Status{State: "ok"})}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		require.True(t, proto.Equal(in, out))
	})

	t.Run("unresolvable payload fails closed", func(t *testing.T) {
		t.Parallel()
		raw, err := proto.Marshal(&pb.Provider{ApiKey: proto.String(secret)})
		require.NoError(t, err)
		in := &pb.Envelope{Payload: &anypb.Any{TypeUrl: "type.googleapis.com/unknown.v1.Secret", Value: raw}}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		require.Equal(t, "type.googleapis.com/google.protobuf.Empty", out.GetPayload().GetTypeUrl())
		require.Empty(t, out.GetPayload().GetValue())
		require.Equal(t, raw, in.GetPayload().GetValue(), "the message itself must not change")
	})

	t.Run("malformed payload fails closed", func(t *testing.T) {
		t.Parallel()
		in := &pb.Envelope{Payload: &anypb.Any{TypeUrl: "type.googleapis.com/redacttest.v1.Provider", Value: []byte{0xff, 0xff, 0xff}}}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		require.Equal(t, "type.googleapis.com/google.protobuf.Empty", out.GetPayload().GetTypeUrl())
		require.Empty(t, out.GetPayload().GetValue())
	})

	t.Run("empty Any is kept", func(t *testing.T) {
		t.Parallel()
		in := &pb.Envelope{Payload: &anypb.Any{}}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		require.True(t, proto.Equal(in, out))
	})
}

// TestDebugRedactAnyValidatesSafeTypes: a payload is decoded before the "no secrets in this type" shortcut, so malformed bytes under a
// type without redacted fields fail closed too, and payloads beyond the shared decode budget become the placeholder.
func TestDebugRedactAnyValidatesSafeTypes(t *testing.T) {
	t.Parallel()

	t.Run("malformed payload of a safe type", func(t *testing.T) {
		t.Parallel()
		in := &pb.Envelope{Payload: &anypb.Any{TypeUrl: "type.googleapis.com/redacttest.v1.Status", Value: []byte{0xff, 0xff, 0xff}}}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		require.Equal(t, "type.googleapis.com/google.protobuf.Empty", out.GetPayload().GetTypeUrl())
		require.Empty(t, out.GetPayload().GetValue())
		_, err := protojson.Marshal(out)
		require.NoError(t, err, "the logged copy stays encodable")
	})

	t.Run("decode budget", func(t *testing.T) {
		t.Parallel()
		big := strings.Repeat("x", 400<<10)
		in := &pb.Envelope{Items: []*anypb.Any{
			mustAny(t, &pb.Status{State: big}), mustAny(t, &pb.Status{State: big}), mustAny(t, &pb.Status{State: big}),
		}}
		out, ok := logger.DebugRedact(in).(*pb.Envelope)
		require.True(t, ok)
		require.True(t, proto.Equal(in.GetItems()[0], out.GetItems()[0]))
		require.True(t, proto.Equal(in.GetItems()[1], out.GetItems()[1]))
		require.Equal(t, "type.googleapis.com/google.protobuf.Empty", out.GetItems()[2].GetTypeUrl(), "beyond 1 MiB of payloads")
	})
}
