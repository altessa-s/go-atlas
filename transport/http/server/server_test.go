// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.readTimeout != DefaultReadTimeout {
		t.Fatalf("readTimeout = %v, want %v", opts.readTimeout, DefaultReadTimeout)
	}
	if opts.writeTimeout != DefaultWriteTimeout {
		t.Fatalf("writeTimeout = %v, want %v", opts.writeTimeout, DefaultWriteTimeout)
	}
	if opts.idleTimeout != DefaultIdleTimeout {
		t.Fatalf("idleTimeout = %v, want %v", opts.idleTimeout, DefaultIdleTimeout)
	}
	if opts.maxHeaderBytes != DefaultMaxHeaderBytes {
		t.Fatalf("maxHeaderBytes = %d, want %d", opts.maxHeaderBytes, DefaultMaxHeaderBytes)
	}
}

func TestWithReadTimeout(t *testing.T) {
	opts := newOptions(WithReadTimeout(30 * time.Second))
	if opts.readTimeout != 30*time.Second {
		t.Fatalf("readTimeout = %v", opts.readTimeout)
	}
}

func TestWithReadTimeout_Negative(t *testing.T) {
	opts := newOptions(WithReadTimeout(-1))
	if opts.readTimeout != DefaultReadTimeout {
		t.Fatalf("negative should keep default, got %v", opts.readTimeout)
	}
}

func TestWithWriteTimeout(t *testing.T) {
	opts := newOptions(WithWriteTimeout(30 * time.Second))
	if opts.writeTimeout != 30*time.Second {
		t.Fatalf("writeTimeout = %v", opts.writeTimeout)
	}
}

func TestWithWriteTimeout_Negative(t *testing.T) {
	opts := newOptions(WithWriteTimeout(-1))
	if opts.writeTimeout != DefaultWriteTimeout {
		t.Fatalf("negative should keep default, got %v", opts.writeTimeout)
	}
}

func TestWithIdleTimeout(t *testing.T) {
	opts := newOptions(WithIdleTimeout(120 * time.Second))
	if opts.idleTimeout != 120*time.Second {
		t.Fatalf("idleTimeout = %v", opts.idleTimeout)
	}
}

func TestWithIdleTimeout_Negative(t *testing.T) {
	opts := newOptions(WithIdleTimeout(-1))
	if opts.idleTimeout != DefaultIdleTimeout {
		t.Fatalf("negative should keep default, got %v", opts.idleTimeout)
	}
}

func TestWithMaxHeaderBytes(t *testing.T) {
	opts := newOptions(WithMaxHeaderBytes(2 << 20))
	if opts.maxHeaderBytes != 2<<20 {
		t.Fatalf("maxHeaderBytes = %d", opts.maxHeaderBytes)
	}
}

func TestWithRouter_Nil(t *testing.T) {
	opts := newOptions(WithRouter(nil))
	if opts.router != nil {
		t.Fatal("nil router should not be set")
	}
}

func TestNew_NoRouter(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("New() without router should error")
	}
}

func TestDefaultConstants(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
	}{
		{"DefaultReadTimeout", DefaultReadTimeout},
		{"DefaultWriteTimeout", DefaultWriteTimeout},
		{"DefaultIdleTimeout", DefaultIdleTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got <= 0 {
				t.Fatalf("%s = %v", tt.name, tt.got)
			}
		})
	}
	if DefaultMaxHeaderBytes <= 0 {
		t.Fatalf("DefaultMaxHeaderBytes = %d", DefaultMaxHeaderBytes)
	}
}
