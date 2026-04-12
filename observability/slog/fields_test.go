// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldsToAttrs_GroupsAndOrder(t *testing.T) {
	fields := Fields{
		{Key: "user.id", Value: 42},
		{Key: "user.name", Value: "alice"},
		{Key: "request.id", Value: "req-1"},
		{Key: "plain", Value: true},
	}

	attrs := FieldsToAttrs(fields)
	require.Len(t, attrs, 3)

	assertAttr(t, attrs[0], "plain", true)

	req := attrs[1]
	require.Equal(t, "request", req.Key)
	require.Equal(t, slog.KindGroup, req.Value.Kind())
	reqGroup := req.Value.Group()
	require.Len(t, reqGroup, 1)
	require.Equal(t, "id", reqGroup[0].Key)
	require.Equal(t, "req-1", reqGroup[0].Value.String())

	user := attrs[2]
	require.Equal(t, "user", user.Key)
	require.Equal(t, slog.KindGroup, user.Value.Kind())
	userGroup := user.Value.Group()
	require.Len(t, userGroup, 2)
	assertAttr(t, userGroup[0], "id", 42)
	assertAttr(t, userGroup[1], "name", "alice")
}

func TestFieldsToAttrs_DuplicateKeepsFirst(t *testing.T) {
	fields := Fields{
		{Key: "plain", Value: "first"},
		{Key: "plain", Value: "second"},
	}

	attrs := FieldsToAttrs(fields)
	require.Len(t, attrs, 1)
	assertAttr(t, attrs[0], "plain", "first")
}

func assertAttr(t *testing.T, attr slog.Attr, wantKey string, wantVal any) {
	t.Helper()
	require.Equal(t, wantKey, attr.Key)

	switch v := wantVal.(type) {
	case bool:
		require.Equal(t, v, attr.Value.Bool())
	case string:
		require.Equal(t, v, attr.Value.String())
	case int:
		require.Equal(t, int64(v), attr.Value.Int64())
	default:
		require.Equal(t, v, attr.Value.Any())
	}
}
