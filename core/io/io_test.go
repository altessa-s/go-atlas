// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

func TestErrorReader(t *testing.T) {
	expectedErr := errors.New("expected error")
	reader := coreio.NewErrorReader(expectedErr)

	buf := make([]byte, 10)
	n, err := reader.Read(buf)

	if n != 0 {
		t.Errorf("Read bytes = %d, want 0", n)
	}
	if err != expectedErr {
		t.Errorf("Read error = %v, want %v", err, expectedErr)
	}
}

func TestLimitedReadCloser(t *testing.T) {
	t.Run("UnderLimit", func(t *testing.T) {
		baseReader := io.NopCloser(strings.NewReader("hello"))
		lrc := coreio.NewLimitedReadCloser(baseReader, 10)

		buf, err := io.ReadAll(lrc)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if string(buf) != "hello" {
			t.Errorf("ReadAll content = %q, want %q", string(buf), "hello")
		}
		if err := lrc.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	t.Run("AtLimit", func(t *testing.T) {
		baseReader := io.NopCloser(strings.NewReader("hello"))
		lrc := coreio.NewLimitedReadCloser(baseReader, 5)

		buf, err := io.ReadAll(lrc)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if string(buf) != "hello" {
			t.Errorf("ReadAll content = %q, want %q", string(buf), "hello")
		}
	})

	t.Run("OverLimit", func(t *testing.T) {
		baseReader := io.NopCloser(strings.NewReader("hello world"))
		lrc := coreio.NewLimitedReadCloser(baseReader, 5)

		_, err := io.ReadAll(lrc)
		if !errors.Is(err, coreio.ErrReadLimitExceeded) {
			t.Errorf("ReadAll error = %v, want %v", err, coreio.ErrReadLimitExceeded)
		}
	})

	t.Run("CloseCallsUnderlying", func(t *testing.T) {
		mrc := &testhelpers.MockReadCloser{Reader: strings.NewReader("test")}
		lrc := coreio.NewLimitedReadCloser(mrc, 100)
		lrc.Close()
		if !mrc.Closed {
			t.Error("Close() did not call underlying Close()")
		}
	})
}

func TestErrorReader_CustomError(t *testing.T) {
	customErr := errors.New("custom read error")
	reader := coreio.NewErrorReader(customErr)

	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	if n != 0 {
		t.Errorf("Read bytes = %d, want 0", n)
	}
	if !errors.Is(err, customErr) {
		t.Errorf("expected custom error, got %v", err)
	}
}

func TestLimitedReadCloser_ZeroLimit(t *testing.T) {
	baseReader := io.NopCloser(strings.NewReader("data"))
	lrc := coreio.NewLimitedReadCloser(baseReader, 0)

	_, err := io.ReadAll(lrc)
	if !errors.Is(err, coreio.ErrReadLimitExceeded) {
		t.Fatalf("expected ErrReadLimitExceeded, got %v", err)
	}
}

func TestLimitedReadCloser_ExactlyOneByte(t *testing.T) {
	baseReader := io.NopCloser(strings.NewReader("x"))
	lrc := coreio.NewLimitedReadCloser(baseReader, 1)

	buf, err := io.ReadAll(lrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(buf) != "x" {
		t.Fatalf("got %q, want %q", string(buf), "x")
	}
}

func TestBufferPool(t *testing.T) {
	buf := coreio.GetBuffer()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}
	buf.WriteString("test")
	coreio.PutBuffer(buf)

	buf2 := coreio.GetBuffer()
	if buf2.Len() != 0 {
		t.Fatal("expected reset buffer from pool")
	}
	coreio.PutBuffer(buf2)
}
