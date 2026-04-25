// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestError_NilReturnsEmpty(t *testing.T) {
	attr := Error(nil)
	require.Empty(t, attr.Key)
}

func TestError_NonNil(t *testing.T) {
	attr := Error(errors.New("fail"))
	require.Equal(t, ErrorKey, attr.Key)
}

func TestString_Value(t *testing.T) {
	attr := String("name", "val")
	require.Equal(t, "name", attr.Key)
	require.Equal(t, "val", attr.Value.String())
}

func TestString_EmptyReturnsEmpty(t *testing.T) {
	attr := String("name", "")
	require.Empty(t, attr.Key)
}

func TestString_Pointer(t *testing.T) {
	v := "hello"
	attr := String("name", &v)
	require.Equal(t, "hello", attr.Value.String())
}

func TestString_NilPointer(t *testing.T) {
	attr := String[*string]("name", nil)
	require.Empty(t, attr.Key)
}

func TestInt_Value(t *testing.T) {
	attr := Int("count", 42)
	require.Equal(t, "count", attr.Key)
	require.Equal(t, int64(42), attr.Value.Int64())
}

func TestInt_Pointer(t *testing.T) {
	v := 42
	attr := Int("count", &v)
	require.Equal(t, int64(42), attr.Value.Int64())
}

func TestInt_NilPointer(t *testing.T) {
	attr := Int[*int]("count", nil)
	require.Empty(t, attr.Key)
}

func TestInt64_Value(t *testing.T) {
	attr := Int64("id", int64(100))
	require.Equal(t, int64(100), attr.Value.Int64())
}

func TestInt64_Int32(t *testing.T) {
	attr := Int64("id", int32(50))
	require.Equal(t, int64(50), attr.Value.Int64())
}

func TestInt64_NilPointers(t *testing.T) {
	attr64 := Int64[*int64]("id", nil)
	require.Empty(t, attr64.Key)

	attr32 := Int64[*int32]("id", nil)
	require.Empty(t, attr32.Key)
}

func TestModule(t *testing.T) {
	attr := Module("http-server")
	require.Equal(t, ModuleKey, attr.Key)
	require.Equal(t, "http-server", attr.Value.String())
}

func TestModuleM(t *testing.T) {
	args := ModuleM("auth", "cache")
	require.Len(t, args, 2)
	for _, arg := range args {
		a, ok := arg.(slog.Attr)
		require.True(t, ok, "expected slog.Attr, got %T", arg)
		require.Equal(t, ModuleKey, a.Key)
	}
}

func TestSetGetLevel(t *testing.T) {
	SetLevel(slog.LevelWarn)
	require.Equal(t, slog.LevelWarn, GetLevel())
	SetLevel(slog.LevelInfo) // restore
}

// shutdownSpy records whether Shutdown was called.
type shutdownSpy struct {
	slog.Handler
	shutdownCalled bool
	shutdownErr    error
}

func (s *shutdownSpy) Shutdown(context.Context) error {
	s.shutdownCalled = true
	return s.shutdownErr
}

// multiSpy implements InnerHandlers for testing fan-out shutdown traversal.
type multiSpy struct {
	slog.Handler
	children []slog.Handler
}

func (m *multiSpy) Handlers() []slog.Handler { return m.children }

func TestShutdown_InnerHandlers(t *testing.T) {
	child1 := &shutdownSpy{Handler: slog.DiscardHandler}
	child2 := &shutdownSpy{Handler: slog.DiscardHandler}
	multi := &multiSpy{
		Handler:  slog.DiscardHandler,
		children: []slog.Handler{child1, child2},
	}
	logger := slog.New(multi)

	require.NoError(t, Shutdown(context.Background(), logger))
	require.True(t, child1.shutdownCalled, "child1 Shutdown not called")
	require.True(t, child2.shutdownCalled, "child2 Shutdown not called")
}

func TestShutdown_InnerHandlers_Error(t *testing.T) {
	errShutdown := errors.New("shutdown failed")
	child1 := &shutdownSpy{Handler: slog.DiscardHandler, shutdownErr: errShutdown}
	child2 := &shutdownSpy{Handler: slog.DiscardHandler}
	multi := &multiSpy{
		Handler:  slog.DiscardHandler,
		children: []slog.Handler{child1, child2},
	}
	logger := slog.New(multi)

	err := Shutdown(context.Background(), logger)
	require.ErrorIs(t, err, errShutdown)
	// child2 should not be called since child1 returned an error.
	require.False(t, child2.shutdownCalled, "child2 Shutdown should not be called after child1 error")
}

func TestMaskingReplaceAttr(t *testing.T) {
	tests := []struct {
		name      string
		sensitive []string
		key       string
		wantMask  bool
	}{
		{"sensitive key", []string{"password"}, "password", true},
		{"case insensitive", []string{"Password"}, "password", true},
		{"not sensitive", []string{"password"}, "username", false},
		{"empty list", nil, "password", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := MaskingReplaceAttr(tt.sensitive, "***")
			attr := slog.String(tt.key, "secret")
			result := fn(nil, attr)
			if tt.wantMask {
				require.Equal(t, "***", result.Value.String())
			} else {
				require.NotEqual(t, "***", result.Value.String(), "should not be masked")
			}
		})
	}
}
