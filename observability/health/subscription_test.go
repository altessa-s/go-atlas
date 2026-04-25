// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscription_InitialStatus(t *testing.T) {
	sub := &Subscription{initialStatus: StatusServing}
	require.Equal(t, StatusServing, sub.InitialStatus())
}

func TestSubscription_Close_Idempotent(t *testing.T) {
	called := 0
	sub := &Subscription{
		cancel: func() { called++ },
	}
	sub.Close()
	sub.Close()
	sub.Close()
	require.Equal(t, 1, called, "cancel should be called exactly once")
}

func TestSubscription_Close_NilCancel(t *testing.T) {
	sub := &Subscription{}
	sub.Close() // should not panic
}

func TestWatcher_Notify(t *testing.T) {
	w := newWatcher(10, StatusUnknown)
	w.notify(StatusServing)

	select {
	case s := <-w.ch:
		require.Equal(t, StatusServing, s)
	default:
		require.Fail(t, "expected notification")
	}
}

func TestWatcher_Notify_AfterClose(t *testing.T) {
	w := newWatcher(1, StatusUnknown)
	w.close()
	w.notify(StatusServing) // should not panic
}

func TestWatcher_Close_Idempotent(t *testing.T) {
	w := newWatcher(1, StatusUnknown)
	w.close()
	w.close() // should not panic
}

func TestWatcher_Notify_FullBuffer(t *testing.T) {
	w := newWatcher(1, StatusUnknown)
	w.notify(StatusServing)    // fills buffer
	w.notify(StatusNotServing) // should not block
}
