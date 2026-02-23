// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
	"time"
)

func TestTimeoutError(t *testing.T) {
	err := &TimeoutError{Signal: syscall.SIGTERM, Timeout: 5 * time.Second}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !IsTimeout(err) {
		t.Fatal("expected IsTimeout to return true")
	}
}

func TestPanicError(t *testing.T) {
	err := &PanicError{Signal: syscall.SIGINT, Panic: "something broke"}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestIsTimeout(t *testing.T) {
	if IsTimeout(fmt.Errorf("regular error")) {
		t.Fatal("expected false for non-timeout error")
	}
	if IsTimeout(nil) {
		t.Fatal("expected false for nil")
	}

	wrapped := fmt.Errorf("wrapped: %w", &TimeoutError{Signal: syscall.SIGTERM, Timeout: time.Second})
	if !IsTimeout(wrapped) {
		t.Fatal("expected true for wrapped timeout error")
	}

	if IsTimeout(errors.New("plain")) {
		t.Fatal("expected false for plain error")
	}
}
