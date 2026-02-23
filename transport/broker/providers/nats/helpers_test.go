// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"errors"
	"testing"
)

func TestIsValidSubject(t *testing.T) {
	tests := []struct {
		subject string
		want    bool
	}{
		{"my-topic", true},
		{"topic123", true},
		{"ABC", true},
		{"a-b-c", true},
		{"topic.with.dots", false},
		{"topic with spaces", false},
		{"topic/slash", false},
		{"", false},
		{"topic*", false},
		{"topic>", false},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			if got := IsValidSubject(tt.subject); got != tt.want {
				t.Fatalf("IsValidSubject(%q) = %v, want %v", tt.subject, got, tt.want)
			}
		})
	}
}

func TestSubjectMatchesPattern(t *testing.T) {
	tests := []struct {
		subject, pattern string
		want             bool
	}{
		// exact match
		{"events.created", "events.created", true},
		{"events.created", "events.deleted", false},
		// single-token wildcard
		{"events.created", "events.*", true},
		{"events.us.created", "events.*", false},
		// multi-token wildcard
		{"orders.created", "orders.>", true},
		{"orders.us.created", "orders.>", true},
		{"orders", "orders.>", false},
		// mixed
		{"a.b.c", "a.*.c", true},
		{"a.b.d", "a.*.c", false},
		{"a.b.c.d", "a.>", true},
		// single token
		{"events", "events", true},
		{"events", "orders", false},
	}

	for _, tt := range tests {
		name := tt.subject + " ~ " + tt.pattern
		t.Run(name, func(t *testing.T) {
			if got := subjectMatchesPattern(tt.subject, tt.pattern); got != tt.want {
				t.Fatalf("subjectMatchesPattern(%q, %q) = %v, want %v", tt.subject, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestCheckSubjectAllowed(t *testing.T) {
	t.Run("no allowlist permits all", func(t *testing.T) {
		n := &Nats{}
		if err := n.checkSubjectAllowed("anything"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("allowed subject passes", func(t *testing.T) {
		n := &Nats{allowedSubjects: []string{"events.*", "orders.>"}}
		if err := n.checkSubjectAllowed("events.created"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := n.checkSubjectAllowed("orders.us.pending"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("disallowed subject rejected", func(t *testing.T) {
		n := &Nats{allowedSubjects: []string{"events.*"}}
		err := n.checkSubjectAllowed("secrets.leak")
		if err == nil {
			t.Fatal("expected error for disallowed subject")
		}
		if !errors.Is(err, ErrSubjectNotAllowed) {
			t.Fatalf("expected ErrSubjectNotAllowed, got: %v", err)
		}
	})
}
