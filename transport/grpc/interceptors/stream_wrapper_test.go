// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"

	grpcmetadata "google.golang.org/grpc/metadata"
)

// mockClientStream implements grpc.ClientStream for testing.
type mockClientStream struct {
	grpc.ClientStream
	ctx     context.Context
	sendErr error
	recvErr error
}

func (m *mockClientStream) Context() context.Context         { return m.ctx }
func (m *mockClientStream) SendMsg(msg any) error            { return m.sendErr }
func (m *mockClientStream) RecvMsg(msg any) error            { return m.recvErr }
func (m *mockClientStream) CloseSend() error                 { return nil }
func (m *mockClientStream) Header() (grpcmetadata.MD, error) { return nil, nil }
func (m *mockClientStream) Trailer() grpcmetadata.MD         { return nil }

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx     context.Context
	sendErr error
	recvErr error
}

func (m *mockServerStream) Context() context.Context         { return m.ctx }
func (m *mockServerStream) SendMsg(msg any) error            { return m.sendErr }
func (m *mockServerStream) RecvMsg(msg any) error            { return m.recvErr }
func (m *mockServerStream) SetHeader(grpcmetadata.MD) error  { return nil }
func (m *mockServerStream) SendHeader(grpcmetadata.MD) error { return nil }
func (m *mockServerStream) SetTrailer(grpcmetadata.MD)       {}

func TestClientStreamWrapper_Context(t *testing.T) {
	ctx := context.WithValue(t.Context(), struct{}{}, "val")
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(ctx, cs, NoopDriver())
	require.Equal(t, ctx, w.Context())
}

func TestClientStreamWrapper_SendMsg(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	err := w.SendMsg("msg")
	require.NoError(t, err)
}

func TestClientStreamWrapper_RecvMsg(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	err := w.RecvMsg(nil)
	require.NoError(t, err)
}

func TestClientStreamWrapper_CloseSend(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	err := w.CloseSend()
	require.NoError(t, err)
}

// A wrapper is reused for a context-only update and when it has no driver
// yet; a second driver gets its own nested wrapper.
func TestClientStreamWrapper_Reuse(t *testing.T) {
	t.Parallel()

	cs := &mockClientStream{ctx: t.Context()}
	ctx2 := context.WithValue(t.Context(), struct{}{}, "v2")

	w1 := NewClientStreamWrapper(t.Context(), cs, nil)
	w2 := NewClientStreamWrapper(ctx2, w1, NoopDriver())
	require.Same(t, w1, w2, "a wrapper without a driver adopts the driver")
	require.Equal(t, ctx2, w2.Context())

	w3 := NewClientStreamWrapper(t.Context(), w2, nil)
	require.Same(t, w2, w3, "a context-only update reuses the wrapper")

	w4 := NewClientStreamWrapper(t.Context(), w3, NoopDriver())
	require.NotSame(t, w3, w4, "a second driver is nested, not dropped")
}

func TestClientStreamWrapper_CloseSendWithoutDriver(t *testing.T) {
	t.Parallel()

	cs := &mockClientStream{ctx: t.Context()}
	require.NoError(t, NewClientStreamWrapper(t.Context(), cs, nil).CloseSend())
}

func TestServerStreamWrapper_Context(t *testing.T) {
	ctx := context.WithValue(t.Context(), struct{}{}, "val")
	ss := &mockServerStream{ctx: t.Context()}
	w := NewServerWrappedStream(ctx, ss, NoopDriver())
	require.Equal(t, ctx, w.Context())
}

func TestServerStreamWrapper_SendMsg(t *testing.T) {
	ss := &mockServerStream{ctx: t.Context()}
	w := NewServerWrappedStream(t.Context(), ss, NoopDriver())
	err := w.SendMsg("msg")
	require.NoError(t, err)
}

func TestServerStreamWrapper_RecvMsg(t *testing.T) {
	ss := &mockServerStream{ctx: t.Context()}
	w := NewServerWrappedStream(t.Context(), ss, NoopDriver())
	err := w.RecvMsg(nil)
	require.NoError(t, err)
}

// A wrapper is reused for a context-only update and when it has no driver
// yet; a second driver gets its own nested wrapper.
func TestServerStreamWrapper_Reuse(t *testing.T) {
	t.Parallel()

	ss := &mockServerStream{ctx: t.Context()}
	ctx2 := context.WithValue(t.Context(), struct{}{}, "v2")

	w1 := NewServerWrappedStream(t.Context(), ss, nil)
	w2 := NewServerWrappedStream(ctx2, w1, NoopDriver())
	require.Same(t, w1, w2, "a wrapper without a driver adopts the driver")
	require.Equal(t, ctx2, w2.Context())

	w3 := NewServerWrappedStream(t.Context(), w2, nil)
	require.Same(t, w2, w3, "a context-only update reuses the wrapper")

	w4 := NewServerWrappedStream(t.Context(), w3, NoopDriver())
	require.NotSame(t, w3, w4, "a second driver is nested, not dropped")
}

// recordingStream records the messages handed to the transport.
type recordingStream struct {
	mockServerStream
	sent []any
}

func (r *recordingStream) SendMsg(m any) error {
	r.sent = append(r.sent, m)
	return r.sendErr
}

// hookDriver records stream hook calls and can replace or reject a message.
type hookDriver struct {
	name    string
	log     *[]string
	replace any
	reject  error
}

func (d *hookDriver) PreCall(context.Context, any) (any, error)          { return nil, nil } //nolint:nilnil // proceed
func (d *hookDriver) PostCall(_ context.Context, _ any, err error) error { return err }
func (d *hookDriver) PostMsgReceive(_ context.Context, _ any, err error) error {
	*d.log = append(*d.log, d.name+":recv")
	return err
}
func (d *hookDriver) PostMsgSent(_ context.Context, m any, err error) error {
	*d.log = append(*d.log, fmt.Sprintf("%s:sent:%v", d.name, m))
	return err
}
func (d *hookDriver) PreMsgSend(_ context.Context, m any) (any, error) {
	*d.log = append(*d.log, fmt.Sprintf("%s:presend:%v", d.name, m))
	if d.reject != nil {
		return nil, d.reject
	}
	if d.replace != nil {
		return d.replace, nil
	}
	return m, nil
}

// Every driver in a chain receives its hooks: pre-send runs from the
// innermost wrapper outwards and sees the replaced message; the transport
// receives the final replacement; receive hooks run outermost first.
func TestServerStreamWrapper_ChainedDrivers(t *testing.T) {
	t.Parallel()

	var log []string
	rs := &recordingStream{mockServerStream: mockServerStream{ctx: t.Context()}}
	outer := NewServerWrappedStream(t.Context(), rs, &hookDriver{name: "outer", log: &log, replace: "outer-copy"})
	inner := NewServerWrappedStream(t.Context(), outer, &hookDriver{name: "inner", log: &log, replace: "inner-copy"})

	require.NoError(t, inner.SendMsg("orig"))
	require.Equal(t, []any{"outer-copy"}, rs.sent)
	require.Equal(t, []string{
		"inner:presend:orig", "outer:presend:inner-copy",
		"outer:sent:outer-copy", "inner:sent:inner-copy",
	}, log)

	log = nil
	require.NoError(t, inner.RecvMsg(nil))
	require.Equal(t, []string{"outer:recv", "inner:recv"}, log)
}

// A pre-send error aborts the send: nothing reaches the transport and no
// post-send hook of that wrapper runs.
func TestServerStreamWrapper_PreSendAbort(t *testing.T) {
	t.Parallel()

	var log []string
	rs := &recordingStream{mockServerStream: mockServerStream{ctx: t.Context()}}
	errReject := errors.New("reject")
	w := NewServerWrappedStream(t.Context(), rs, &hookDriver{name: "d", log: &log, reject: errReject})

	require.ErrorIs(t, w.SendMsg("orig"), errReject)
	require.Empty(t, rs.sent)
	require.Equal(t, []string{"d:presend:orig"}, log)
}

// Transport errors still reach the post-send hook and the caller.
func TestServerStreamWrapper_SendErrorPropagates(t *testing.T) {
	t.Parallel()

	var log []string
	errSend := errors.New("send failed")
	rs := &recordingStream{mockServerStream: mockServerStream{ctx: t.Context(), sendErr: errSend}}
	w := NewServerWrappedStream(t.Context(), rs, &hookDriver{name: "d", log: &log})

	require.ErrorIs(t, w.SendMsg("m"), errSend)
	require.Equal(t, []string{"d:presend:m", "d:sent:m"}, log)
}

// recordingClientStream records the messages handed to the transport.
type recordingClientStream struct {
	mockClientStream
	sent   []any
	closed int
}

func (r *recordingClientStream) SendMsg(m any) error {
	r.sent = append(r.sent, m)
	return r.sendErr
}

func (r *recordingClientStream) CloseSend() error {
	r.closed++
	return nil
}

// closeDriver records PostCall invocations from CloseSend.
type closeDriver struct {
	hookDriver
}

func (d *closeDriver) PostCall(_ context.Context, _ any, err error) error {
	*d.log = append(*d.log, d.name+":close")
	return err
}

// On a client, wrappers are applied as the streamer returns: the outermost
// interceptor's wrapper is innermost-created last, so its pre-send hook runs
// first; every driver sees PostCall on CloseSend.
func TestClientStreamWrapper_ChainedDrivers(t *testing.T) {
	t.Parallel()

	var log []string
	rs := &recordingClientStream{mockClientStream: mockClientStream{ctx: t.Context()}}
	inner := NewClientStreamWrapper(t.Context(), rs, &closeDriver{hookDriver{name: "inner", log: &log, replace: "inner-copy"}})
	outer := NewClientStreamWrapper(t.Context(), inner, &closeDriver{hookDriver{name: "outer", log: &log, replace: "outer-copy"}})

	require.NoError(t, outer.SendMsg("orig"))
	require.Equal(t, []any{"inner-copy"}, rs.sent)
	require.Equal(t, []string{
		"outer:presend:orig", "inner:presend:outer-copy",
		"inner:sent:inner-copy", "outer:sent:outer-copy",
	}, log)

	log = nil
	require.NoError(t, outer.CloseSend())
	require.Equal(t, 1, rs.closed)
	require.Equal(t, []string{"inner:close", "outer:close"}, log)
}

// A client pre-send error aborts the send.
func TestClientStreamWrapper_PreSendAbort(t *testing.T) {
	t.Parallel()

	var log []string
	rs := &recordingClientStream{mockClientStream: mockClientStream{ctx: t.Context()}}
	errReject := errors.New("reject")
	w := NewClientStreamWrapper(t.Context(), rs, &hookDriver{name: "d", log: &log, reject: errReject})

	require.ErrorIs(t, w.SendMsg("orig"), errReject)
	require.Empty(t, rs.sent)
	require.Equal(t, []string{"d:presend:orig"}, log)
}
