// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// This test replaces the global slog logger, so it does not run in parallel.
func TestLeader_CallbackPanicIsLoggedWithoutValue(t *testing.T) {
	var logs lockedBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	const secret = "private-callback-payload"
	done := make(chan struct{})
	prov := &mockProvider{}
	prov.startFn = func(_ context.Context, cfg providers.Config) error {
		prov.isLeader = true
		cfg.BecameCh <- struct{}{}
		return nil
	}
	le := leadelect.New(prov, "election", "node-1")
	t.Cleanup(func() { require.NoError(t, le.Stop(t.Context())) })
	le.RegisterOnBecomesLeader(func(context.Context, leadelect.LeaderElector) {
		defer close(done)
		panic(secret)
	})
	require.NoError(t, le.Start(t.Context()))
	<-done
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "category=worker_panic")
	}, 5e9, 1e7)
	require.NotContains(t, logs.String(), secret)
}
