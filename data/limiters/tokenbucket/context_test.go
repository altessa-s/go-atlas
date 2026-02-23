// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func TestContextWithClientIP(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := tokenbucket.ContextWithClientIP(t.Context(), "1.2.3.4")
		got := tokenbucket.ExtractClientIp(ctx)
		if got != "1.2.3.4" {
			t.Errorf("ExtractClientIp() = %q, want %q", got, "1.2.3.4")
		}
	})

	t.Run("empty context", func(t *testing.T) {
		got := tokenbucket.ExtractClientIp(t.Context())
		if got != "" {
			t.Errorf("ExtractClientIp() = %q, want empty", got)
		}
	})

	t.Run("empty ip", func(t *testing.T) {
		ctx := tokenbucket.ContextWithClientIP(t.Context(), "")
		got := tokenbucket.ExtractClientIp(ctx)
		if got != "" {
			t.Errorf("ExtractClientIp() = %q, want empty", got)
		}
	})
}

func TestContextWithAuthToken(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := tokenbucket.ContextWithAuthToken(t.Context(), "my-token")
		got := tokenbucket.ExtractAuthToken(ctx)
		if got != "my-token" {
			t.Errorf("ExtractAuthToken() = %q, want %q", got, "my-token")
		}
	})

	t.Run("empty context", func(t *testing.T) {
		got := tokenbucket.ExtractAuthToken(t.Context())
		if got != "" {
			t.Errorf("ExtractAuthToken() = %q, want empty", got)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		ctx := tokenbucket.ContextWithAuthToken(t.Context(), "")
		got := tokenbucket.ExtractAuthToken(ctx)
		if got != "" {
			t.Errorf("ExtractAuthToken() = %q, want empty", got)
		}
	})
}
