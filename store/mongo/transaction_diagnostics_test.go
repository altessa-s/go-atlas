// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransactionDiagnosticsRedactPayload(t *testing.T) {
	t.Parallel()

	const marker = "private transaction payload"
	for _, panics := range []bool{false, true} {
		name := "callback error"
		if panics {
			name = "callback panic"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			m := connectMockMongo(t, WithLogger(slog.New(slog.NewTextHandler(&output, nil))))

			callbackErr := errors.New(marker)
			err := m.WithTransaction(t.Context(), func(context.Context) error {
				if panics {
					panic(marker)
				}
				return callbackErr
			})
			if panics {
				require.ErrorIs(t, err, ErrTransactionPanicked)
				require.NotContains(t, err.Error(), marker)
			} else {
				require.ErrorIs(t, err, callbackErr)
			}
			require.Contains(t, output.String(), "transaction failed")
			require.NotContains(t, output.String(), marker)
			if panics {
				require.Contains(t, output.String(), "stacktrace=")
				require.Contains(t, output.String(), "error_class=transaction_panicked")
			} else {
				require.Contains(t, output.String(), "error_class=callback_error")
			}
		})
	}
}
