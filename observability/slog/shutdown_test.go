// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/buffered"
	"github.com/altessa-s/go-atlas/observability/slog/handler/multi"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// recorder collects record messages; handlers derived from it share the log.
type recorder struct {
	mu   *sync.Mutex
	msgs *[]string
}

func newRecorder() recorder { return recorder{mu: new(sync.Mutex), msgs: new([]string)} }

func (r recorder) Enabled(context.Context, slog.Level) bool { return true }
func (r recorder) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r recorder) WithGroup(string) slog.Handler            { return r }

func (r recorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	*r.msgs = append(*r.msgs, rec.Message)
	return nil
}

func (r recorder) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), *r.msgs...)
}

// Shutdown must reach buffered children through multi.Handler, including the
// fresh children a derived logger's With/WithGroup creates.
func TestShutdown_DrainsBufferedChildrenOfMultiHandler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mh   func(...slog.Handler) *multi.Handler
	}{
		{"sequential", multi.NewHandler},
		{"concurrent", multi.NewConcurrentHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := newRecorder()
			root := slog.New(tc.mh(buffered.NewHandler(sink, buffered.WithBufferSize(64))))
			withAttrs := root.With("k", "v")
			withGroup := root.WithGroup("g")

			root.Info("root")
			withAttrs.Info("with-attrs")
			withGroup.Info("with-group")

			for _, l := range []*slog.Logger{root, withAttrs, withGroup} {
				require.NoError(t, slogx.Shutdown(t.Context(), l))
			}
			require.ElementsMatch(t, []string{"root", "with-attrs", "with-group"}, sink.messages())
		})
	}
}
