// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponse_JSON_Data(t *testing.T) {
	resp := Response{Data: "hello"}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(b, &result))
	require.Equal(t, "hello", result["data"])
	_, ok := result["error"]
	require.False(t, ok, "error should be omitted")
}

func TestResponse_JSON_Error(t *testing.T) {
	resp := Response{Error: &Error{Code: "NOT_FOUND", Message: "not found"}}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(b, &result))
	_, ok := result["data"]
	require.False(t, ok, "data should be omitted")
	errMap := result["error"].(map[string]any)
	require.Equal(t, "NOT_FOUND", errMap["code"])
}

func TestResponse_JSON_Empty(t *testing.T) {
	resp := Response{}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	// Both data and error should be omitted
	var result map[string]any
	json.Unmarshal(b, &result)
	require.Len(t, result, 0)
}

func TestError_JSON(t *testing.T) {
	e := Error{Code: "TEST", Message: "msg"}
	b, err := json.Marshal(e)
	require.NoError(t, err)
	var result map[string]any
	json.Unmarshal(b, &result)
	require.Equal(t, "TEST", result["code"])
	require.Equal(t, "msg", result["message"])
}
