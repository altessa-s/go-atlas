// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
)

// TestBuild_HandsTheLoggerToTheStorage pins that the storage
// function receives the builder's logger: the one set with UseLogger, and the
// discarding default otherwise — never nil.
func TestBuild_HandsTheLoggerToTheStorage(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(t.Output(), nil))
	tests := []struct {
		name   string
		logger *slog.Logger
	}{
		{name: "logger set", logger: logger},
		{name: "default logger"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got *slog.Logger
			capture := func(_ context.Context, _ *brokerconfig.Outbox, l *slog.Logger) (outbox.Storage, error) {
				got = l
				return nil, errStorage
			}

			b := New(&brokerconfig.Outbox{Enabled: true}).UseStorage(brokerconfig.OutboxStorageTypeMongo, capture)
			if tc.logger != nil {
				b.UseLogger(tc.logger)
			}
			_, err := b.Build(t.Context(), noopHandler)
			require.ErrorIs(t, err, errStorage)

			require.NotNil(t, got)
			if tc.logger != nil {
				require.Same(t, tc.logger, got)
			}
		})
	}
}
