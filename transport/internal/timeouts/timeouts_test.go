// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeouts

import (
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"StartupVerification", cfg.StartupVerification, DefaultStartupVerification},
		{"ShutdownGraceful", cfg.ShutdownGraceful, DefaultShutdownGraceful},
		{"ShutdownWarning", cfg.ShutdownWarning, DefaultShutdownWarning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("Default().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultStartupVerification != 2*time.Second {
		t.Fatalf("DefaultStartupVerification = %v, want 2s", DefaultStartupVerification)
	}
	if DefaultShutdownGraceful != time.Minute {
		t.Fatalf("DefaultShutdownGraceful = %v, want 1m", DefaultShutdownGraceful)
	}
	if DefaultShutdownWarning != 15*time.Second {
		t.Fatalf("DefaultShutdownWarning = %v, want 15s", DefaultShutdownWarning)
	}
}
