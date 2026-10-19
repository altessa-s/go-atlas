// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/idempotency"
)

func TestDecodeRecord(t *testing.T) {
	t.Parallel()

	_, ok := idempotency.DecodeRecord(nil)
	require.False(t, ok)
	_, ok = idempotency.DecodeRecord("")
	require.False(t, ok)

	legacy, ok := idempotency.DecodeRecord("entity-1")
	require.True(t, ok)
	require.Equal(t, idempotency.Record{EntityID: "entity-1"}, legacy, "a bare string is a pre-replay entity ID")
	require.False(t, legacy.HasGRPCResponse(""))
	require.False(t, legacy.HasHTTPResponse(""))

	// The Keeper's JSON serializer hands a stored record back as a map.
	want := idempotency.Record{EntityID: "e", Binding: "POST /o", Response: []byte{1, 2}, Status: 201,
		Header: map[string][]string{"Location": {"/o/1"}}, Body: []byte("{}")}
	raw, err := json.Marshal(want)
	require.NoError(t, err)
	require.JSONEq(t, `{"entity_id":"e","binding":"POST /o","response":"AQI=","status":201,"header":{"Location":["/o/1"]},"body":"e30="}`, string(raw),
		"the persisted field names are frozen")
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	got, ok := idempotency.DecodeRecord(decoded)
	require.True(t, ok)
	require.Equal(t, want, got)
	require.True(t, got.HasGRPCResponse("POST /o"))
	require.True(t, got.HasHTTPResponse("POST /o"))
	require.False(t, got.HasHTTPResponse("POST /p"), "another binding")
	unbound := got
	unbound.Binding = ""
	require.False(t, unbound.HasHTTPResponse(""), "a record without a binding is never replayed")

	same, ok := idempotency.DecodeRecord(want)
	require.True(t, ok)
	require.Equal(t, want, same)

	_, ok = idempotency.DecodeRecord(map[string]any{"status": "not a number"})
	require.False(t, ok, "a value that is not a record")
	_, ok = idempotency.DecodeRecord(func() {})
	require.False(t, ok)
}
