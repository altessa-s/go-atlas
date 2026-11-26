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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
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
			opts := options.Client()
			opts.Deployment = drivertest.NewMockDeployment(bson.D{{Key: "ok", Value: 1}})
			client, err := mongo.Connect(opts)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
			m, err := New("db", WithLogger(slog.New(slog.NewTextHandler(&output, nil))))
			require.NoError(t, err)

			callbackErr := errors.New(marker)
			err = m.conn.withTransaction(t.Context(), client, func(context.Context) error {
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
		})
	}
}
