// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSignal_ZeroTimeoutHandler_UsesGlobalCtx(t *testing.T) {
	s := New(WithHandlerTimeout(0))

	started := make(chan struct{})
	done := make(chan struct{})

	entry := s.newHandlerEntry(func(ctx context.Context, sig os.Signal) error {
		close(started)
		<-ctx.Done()
		return nil
	})

	go func() {
		s.executeHandlerEntryOptimized(entry, syscall.SIGTERM)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("handler did not start")
	}

	s.globalCancel()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("handler did not stop after global cancel")
	}
}
