// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsle

import (
	"log/slog"
	"net/http"
	"testing"
)

func TestHostFromAddr(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{":80", ""},
		{"0.0.0.0:80", "0.0.0.0"},
		{"127.0.0.1", "127.0.0.1"},
		{"[::]:80", "::"},
		{"[::]", "::"},
		{"::", "::"},
	}

	for _, tt := range tests {
		if got := hostFromAddr(tt.in); got != tt.want {
			t.Fatalf("hostFromAddr(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStartHTTPServerWithContext_NilContext(t *testing.T) {
	le := &LetsEncrypt{opts: &options{logger: slog.New(slog.DiscardHandler)}}
	if err := le.StartHTTPServerWithContext(nil, ":80", nil); err == nil {
		t.Fatalf("expected error for nil ctx")
	}
}

func TestStartHTTPServerWithContext_AlreadyStarted(t *testing.T) {
	le := &LetsEncrypt{opts: &options{logger: slog.New(slog.DiscardHandler)}}
	le.mu.Lock()
	le.httpSrv = &http.Server{}
	le.mu.Unlock()

	if err := le.StartHTTPServerWithContext(t.Context(), ":80", nil); err == nil {
		t.Fatalf("expected already-started error")
	}
}
