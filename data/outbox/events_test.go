// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestEvent_NextAttempt(t *testing.T) {
	e := &Event{Attempts: 0}
	e.nextAttempt()
	require.Equal(t, uint32(1), e.Attempts)
	require.False(t, e.LastAttemptOn.IsZero(), "LastAttemptOn is zero")
	require.True(t, e.LockedOn.IsZero(), "LockedOn should be cleared")
}

func TestEvent_SetErrorStatus(t *testing.T) {
	e := &Event{}
	e.setErrorStatus(errors.New("some error"))
	require.Equal(t, StatusFailed, e.Status)
	require.NotNil(t, e.LastError)
	require.Equal(t, "some error", *e.LastError)
}

func TestEvent_SetErrorStatus_ContextCanceled(t *testing.T) {
	e := &Event{}
	e.setErrorStatus(context.Canceled)
	require.Equal(t, StatusFailed, e.Status)
	require.Nil(t, e.LastError)
}

func TestEvent_SetSentStatus(t *testing.T) {
	e := &Event{LastError: testhelpers.StringPtr("old error")}
	e.setSentStatus()
	require.Equal(t, StatusSent, e.Status)
	require.Nil(t, e.LastError)
	require.False(t, e.PublishedAt.IsZero(), "PublishedAt is zero")
}

func TestEvent_SetSkippedStatus(t *testing.T) {
	e := &Event{LastError: testhelpers.StringPtr("old error")}
	e.setSkippedStatus()
	require.Equal(t, StatusSkipped, e.Status)
	require.Nil(t, e.LastError)
	require.False(t, e.PublishedAt.IsZero(), "PublishedAt is zero")
}

func TestEvent_SetStatusMaxAttemptReached(t *testing.T) {
	e := &Event{}
	e.setStatusMaxAttemptReached()
	require.Equal(t, StatusMaxAttemptReached, e.Status)
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
			got := e.isReadyForRetry(tt.maxAttempts)
			require.Equal(t, tt.want, got)
		})
	}
}
