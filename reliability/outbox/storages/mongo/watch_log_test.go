// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// captureHandler records every log record it is handed.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *captureHandler) WithGroup(string) slog.Handler            { return h }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

// byLevel returns the recorded entries of one level, each rendered as its
// message followed by its attributes.
func (h *captureHandler) byLevel(level slog.Level) []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	var out []string
	for _, r := range h.records {
		if r.Level != level {
			continue
		}
		var b strings.Builder
		b.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			b.WriteString(" " + a.String())
			return true
		})
		out = append(out, b.String())
	}
	return out
}

// A deployment that never answers must be visible without flooding the log:
// the first failure is warned about, then every watchLogEvery-th.
func TestLogProbe_WarnsAtABoundedRate(t *testing.T) {
	t.Parallel()

	const failures = 3*watchLogEvery + 5

	h := &captureHandler{}
	calls := 0
	probe := logProbe(slog.New(h), func(context.Context) (bool, error) {
		calls++
		if calls <= failures {
			return false, errProbe
		}
		return true, nil
	})

	supported, err := probeUntilAnswered(t.Context(), fastBackoff(), probe)
	require.NoError(t, err)
	require.True(t, supported)
	require.Equal(t, failures+1, calls)

	require.Len(t, h.byLevel(slog.LevelWarn), 1+failures/watchLogEvery)
	require.Len(t, h.byLevel(slog.LevelInfo), 1)
}

// An answer that follows no failure is the normal start and logs nothing, and
// neither does a probe cut short by the watcher's own shutdown.
func TestLogProbe_QuietWithoutFailures(t *testing.T) {
	t.Parallel()

	h := &captureHandler{}
	answered := logProbe(slog.New(h), func(context.Context) (bool, error) { return false, nil })
	_, err := answered(t.Context())
	require.NoError(t, err)

	canceled := logProbe(slog.New(h), func(context.Context) (bool, error) { return false, context.Canceled })
	_, err = canceled(t.Context())
	require.ErrorIs(t, err, context.Canceled)

	require.Empty(t, h.records)
}

// The warning carries the driver's error, which names the unreachable server
// but never the connection string or its credentials.
func TestLogProbe_DoesNotLogTheConnectionString(t *testing.T) {
	t.Parallel()

	const (
		password = "s3cr3t-outbox-password"
		uri      = "mongodb://outbox-user:" + password + "@127.0.0.1:1/?authSource=admin"
	)

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(uri).SetServerSelectionTimeout(100 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

	h := &captureHandler{}
	s := &Store{collection: client.Database("outbox").Collection(DefaultCollectionName), logger: slog.New(h)}

	_, err = logProbe(s.logger, s.SupportsChangeStreams)(t.Context())
	require.Error(t, err)

	warns := h.byLevel(slog.LevelWarn)
	require.Len(t, warns, 1)
	for _, forbidden := range []string{uri, password, "outbox-user", "mongodb://"} {
		require.NotContains(t, warns[0], forbidden)
	}
}

// The store's default logger discards, so a store built without WithLogger
// logs nowhere and never dereferences a nil logger.
func TestOptions_LoggerDefaultsToDiscard(t *testing.T) {
	t.Parallel()

	opts := newOptions(WithLogger(nil))
	require.NotNil(t, opts.logger)
	require.False(t, opts.logger.Enabled(t.Context(), slog.LevelError))

	h := &captureHandler{}
	require.Same(t, h, newOptions(WithLogger(slog.New(h))).logger.Handler())
}
