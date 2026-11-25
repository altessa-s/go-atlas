// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authaudit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/transport/internal/authaudit"
)

func TestRecordAuthentication(t *testing.T) {
	t.Parallel()
	subjectOf := func(p string) string { require.FailNow(t, "unexpected subject lookup"); return p }
	require.NoError(t, authaudit.RecordAuthentication(t.Context(), nil, "http", "authenticate", "", subjectOf, true, ""))
	sink := decision.SinkFunc(func(_ context.Context, d decision.Decision) error {
		require.False(t, d.Allowed)
		require.Empty(t, d.Subject)
		require.Equal(t, "invalid_token", d.Reason)
		require.Equal(t, "authenticate", d.Action)
		require.Equal(t, map[string]string{"transport": "http"}, d.Attributes)
		require.False(t, d.Time.IsZero())
		return decision.ErrAuditFailed
	})
	require.ErrorIs(t, authaudit.RecordAuthentication(t.Context(), sink, "http", "authenticate", "", subjectOf, false, "invalid_token"), decision.ErrAuditFailed)
}
