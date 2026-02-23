// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"log/slog"
	"testing"
)

func TestDefaultWriterOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.defaultCodec != DefaultCodecJSON {
		t.Fatalf("defaultCodec = %q, want %q", opts.defaultCodec, DefaultCodecJSON)
	}
	if !opts.fallbackOnNegotiationError {
		t.Fatal("fallbackOnNegotiationError should default to true")
	}
	if opts.registry == nil {
		t.Fatal("registry is nil")
	}
	if opts.responseBuilder == nil {
		t.Fatal("responseBuilder is nil")
	}
}

func TestWithDefaultCodec(t *testing.T) {
	opts := newOptions(WithDefaultCodec("application/xml"))
	if opts.defaultCodec != "application/xml" {
		t.Fatalf("defaultCodec = %q", opts.defaultCodec)
	}
}

func TestWithDefaultCodec_StringPtr(t *testing.T) {
	s := "text/plain"
	opts := newOptions(WithDefaultCodec(&s))
	if opts.defaultCodec != "text/plain" {
		t.Fatalf("defaultCodec = %q", opts.defaultCodec)
	}
}

func TestWithDefaultCodec_NilPtr(t *testing.T) {
	opts := newOptions(WithDefaultCodec[*string](nil))
	if opts.defaultCodec != DefaultCodecJSON {
		t.Fatalf("nil ptr should keep default, got %q", opts.defaultCodec)
	}
}

func TestWithDefaultCodec_Trimmed(t *testing.T) {
	opts := newOptions(WithDefaultCodec("  application/json  "))
	if opts.defaultCodec != "application/json" {
		t.Fatalf("defaultCodec = %q", opts.defaultCodec)
	}
}

func TestWithMaxBodySize(t *testing.T) {
	opts := newOptions(WithMaxBodySize(1024))
	if opts.maxBodySize != 1024 {
		t.Fatalf("maxBodySize = %d", opts.maxBodySize)
	}
}

func TestWithWriterLogger(t *testing.T) {
	l := slog.New(slog.DiscardHandler)
	opts := newOptions(WithLogger(l))
	if opts.logger != l {
		t.Fatal("logger not set")
	}
}

func TestWithWriterLogger_Nil(t *testing.T) {
	opts := newOptions(WithLogger(nil))
	if opts.logger == nil {
		t.Fatal("nil logger should keep default")
	}
}

func TestWithRegistry_Nil(t *testing.T) {
	opts := newOptions(WithRegistry(nil))
	if opts.registry == nil {
		t.Fatal("nil registry should keep default")
	}
}

func TestWithErrorConverter(t *testing.T) {
	converter := func(err error) (Error, int) {
		return Error{}, 0
	}
	opts := newOptions(WithErrorConverter(converter))
	if opts.errorConverter == nil {
		t.Fatal("errorConverter not set")
	}
}

func TestWithFallbackOnNegotiationError(t *testing.T) {
	opts := newOptions(WithFallbackOnNegotiationError())
	if !opts.fallbackOnNegotiationError {
		t.Fatal("should be true")
	}
}
