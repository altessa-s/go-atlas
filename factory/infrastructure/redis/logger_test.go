// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlogAdapter_ReportsDiagnosticWithoutRawText(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	adapter := NewSlogAdapter(slog.New(slog.NewJSONHandler(&output, nil)))
	adapter.Printf(t.Context(), "Secret: %s", "PrivateAddress")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	require.Equal(t, "WARN", entry[slog.LevelKey])
	require.NotContains(t, output.String(), "Secret")
	require.NotContains(t, output.String(), "PrivateAddress")
}
