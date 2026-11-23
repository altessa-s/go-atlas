// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/msg"
)

func TestPublishOmitsOutboxOrderKeyHeader(t *testing.T) {
	t.Parallel()
	wire, _ := buildNatsMsg(msg.Message{Metadata: msg.Meta{
		{Key: msg.MetaKeyOrderKey, Value: "order-5"},
		{Key: "region", Value: "west"},
	}})
	require.NotContains(t, wire.Header, msg.MetaKeyOrderKey)
	require.Equal(t, "west", wire.Header.Get("region"))
}

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
			got := IsValidSubject(tt.subject)
			require.Equal(t, tt.want, got)
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
			got := subjectMatchesPattern(tt.subject, tt.pattern)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCheckSubjectAllowed(t *testing.T) {
	t.Run("no allowlist permits all", func(t *testing.T) {
		n := &NATS{}
		err := n.checkSubjectAllowed("anything")
		require.NoError(t, err)
	})

	t.Run("allowed subject passes", func(t *testing.T) {
		n := &NATS{allowedSubjects: []string{"events.*", "orders.>"}}
		err := n.checkSubjectAllowed("events.created")
		require.NoError(t, err)
		err = n.checkSubjectAllowed("orders.us.pending")
		require.NoError(t, err)
	})

	t.Run("disallowed subject rejected", func(t *testing.T) {
		n := &NATS{allowedSubjects: []string{"events.*"}}
		err := n.checkSubjectAllowed("secrets.leak")
		require.NotNil(t, err, "expected error for disallowed subject")
		require.True(t, errors.Is(err, ErrSubjectNotAllowed), "expected ErrSubjectNotAllowed, got: %v", err)
	})
}
