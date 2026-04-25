// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ping

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

func TestHandler(t *testing.T) {
	w := writer.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ping", nil)

	rw := writer.NewReadWriter(rec, req, w)
	Handler(rw)
	rw.Release()

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data Response `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "pong", resp.Data.Message)
}

func TestResponse_JSON(t *testing.T) {
	resp := Response{Message: "pong"}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	var result map[string]string
	require.NoError(t, json.Unmarshal(b, &result))
	require.Equal(t, "pong", result["message"])
}
