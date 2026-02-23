// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEvent_NextAttempt(t *testing.T) {
	e := &Event{Attempts: 0}
	e.nextAttempt()
	if e.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", e.Attempts)
	}
	if e.LastAttemptOn.IsZero() {
		t.Fatal("LastAttemptOn is zero")
	}
	if !e.LockedOn.IsZero() {
		t.Fatal("LockedOn should be cleared")
	}
}

func TestEvent_SetErrorStatus(t *testing.T) {
	e := &Event{}
	e.setErrorStatus(errors.New("some error"))
	if e.Status != StatusFailed {
		t.Fatalf("Status = %q, want %q", e.Status, StatusFailed)
	}
	if e.LastError == nil || *e.LastError != "some error" {
		t.Fatalf("LastError = %v", e.LastError)
	}
}

func TestEvent_SetErrorStatus_ContextCanceled(t *testing.T) {
	e := &Event{}
	e.setErrorStatus(context.Canceled)
	if e.Status != StatusFailed {
		t.Fatalf("Status = %q", e.Status)
	}
	if e.LastError != nil {
		t.Fatalf("LastError should be nil for context.Canceled, got %v", *e.LastError)
	}
}

func TestEvent_SetSentStatus(t *testing.T) {
	e := &Event{LastError: ptrStr("old error")}
	e.setSentStatus()
	if e.Status != StatusSent {
		t.Fatalf("Status = %q", e.Status)
	}
	if e.LastError != nil {
		t.Fatal("LastError should be nil")
	}
	if e.PublishedAt.IsZero() {
		t.Fatal("PublishedAt is zero")
	}
}

func TestEvent_SetStatusMaxAttemptReached(t *testing.T) {
	e := &Event{}
	e.setStatusMaxAttemptReached()
	if e.Status != StatusMaxAttemptReached {
		t.Fatalf("Status = %q", e.Status)
	}
}

func TestEvent_IsReadyForRetry(t *testing.T) {
	tests := []struct {
		name        string
		attempts    uint32
		maxAttempts uint32
		want        bool
	}{
		{"below_max", 3, 10, true},
		{"at_max", 10, 10, false},
		{"above_max", 11, 10, false},
		{"zero", 0, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Event{Attempts: tt.attempts}
			if got := e.isReadyForRetry(tt.maxAttempts); got != tt.want {
				t.Fatalf("isReadyForRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusInProgress, "in-progress"},
		{StatusPending, "pending"},
		{StatusSent, "sent"},
		{StatusFailed, "failed"},
		{StatusMaxAttemptReached, "max-attempt-reached"},
		{StatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Fatalf("Status = %q, want %q", tt.status, tt.want)
			}
		})
	}
}

func TestOptionConstants(t *testing.T) {
	if DefaultEventsBatchSize != 200 {
		t.Fatalf("DefaultEventsBatchSize = %d", DefaultEventsBatchSize)
	}
	if DefaultRetryMaxAttempts != 10 {
		t.Fatalf("DefaultRetryMaxAttempts = %d", DefaultRetryMaxAttempts)
	}
	if DefaultFetchTimeout != 5*time.Second {
		t.Fatalf("DefaultFetchTimeout = %v", DefaultFetchTimeout)
	}
	if DefaultHandleTimeout != 20*time.Second {
		t.Fatalf("DefaultHandleTimeout = %v", DefaultHandleTimeout)
	}
	if DefaultLockInterval != 10*time.Second {
		t.Fatalf("DefaultLockInterval = %v", DefaultLockInterval)
	}
}

func TestErrSchedulerManaged(t *testing.T) {
	if ErrSchedulerManaged == nil {
		t.Fatal("ErrSchedulerManaged is nil")
	}
}

func ptrStr(s string) *string { return &s }
