// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// resourceWithSecrets returns a proto Resource populated with INPUT_ONLY fields
// (Password at the top level and Profile.Secret nested) alongside ordinary
// fields that must survive sanitization.
func resourceWithSecrets() *testpb.Resource {
	return &testpb.Resource{
		Name:        "public-name",
		Description: "public-description",
		Password:    "top-secret",
		Profile: &testpb.Profile{
			DisplayName: "dn",
			Secret:      "nested-secret",
		},
	}
}

// writeJSON runs data through a default Writer and returns the decoded JSON body.
func writeJSON(t *testing.T, wr *writer.Writer, data any) map[string]any {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	require.NoError(t, wr.Write(rec, req, data))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

// dataField digs the handler payload out of the response envelope, tolerating
// either a "data"-wrapped envelope or a flat object.
func dataField(body map[string]any) map[string]any {
	if d, ok := body["data"].(map[string]any); ok {
		return d
	}
	return body
}

// TestWrite_StripsInputOnly_ByDefault pins the on-by-default behavior: a proto
// response with populated INPUT_ONLY fields is emitted without them.
func TestWrite_StripsInputOnly_ByDefault(t *testing.T) {
	t.Parallel()

	wr := writer.New()
	data := dataField(writeJSON(t, wr, resourceWithSecrets()))

	require.Equal(t, "public-name", data["name"])
	require.NotContains(t, data, "password", "top-level INPUT_ONLY must be stripped")

	profile, ok := data["profile"].(map[string]any)
	require.True(t, ok, "profile must be present")
	require.Equal(t, "dn", profile["display_name"])
	require.NotContains(t, profile, "secret", "nested INPUT_ONLY must be stripped")
}

// TestWrite_DoesNotMutateOriginal proves the handler's message is untouched: the
// clone-on-detect path strips only the copy that gets encoded.
func TestWrite_DoesNotMutateOriginal(t *testing.T) {
	t.Parallel()

	original := resourceWithSecrets()
	wr := writer.New()
	_ = writeJSON(t, wr, original)

	require.Equal(t, "top-secret", original.GetPassword(), "original must keep INPUT_ONLY")
	require.Equal(t, "nested-secret", original.GetProfile().GetSecret())
}

// TestWrite_SanitizationDisabled confirms the opt-out emits INPUT_ONLY fields.
func TestWrite_SanitizationDisabled(t *testing.T) {
	t.Parallel()

	wr := writer.New(writer.WithResponseSanitizationDisabled())
	data := dataField(writeJSON(t, wr, resourceWithSecrets()))

	require.Equal(t, "top-secret", data["password"], "disabled: INPUT_ONLY must pass through")
}

// TestWrite_NonProtoUnchanged verifies non-proto payloads bypass sanitization
// unchanged.
func TestWrite_NonProtoUnchanged(t *testing.T) {
	t.Parallel()

	wr := writer.New()
	payload := map[string]any{"name": "n", "password": "kept"}
	data := dataField(writeJSON(t, wr, payload))

	require.Equal(t, "n", data["name"])
	require.Equal(t, "kept", data["password"], "non-proto data must not be sanitized")
}

// TestWrite_NoInputOnly_NotAltered checks a proto response without populated
// INPUT_ONLY fields round-trips intact.
func TestWrite_NoInputOnly_NotAltered(t *testing.T) {
	t.Parallel()

	wr := writer.New()
	data := dataField(writeJSON(t, wr, &testpb.Resource{Name: "only-public"}))

	require.Equal(t, "only-public", data["name"])
}

// TestWriteStream_StripsInputOnly pins that the streaming path sanitizes too.
func TestWriteStream_StripsInputOnly(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	wr := writer.New()
	require.NoError(t, wr.WriteStream(rec, req, resourceWithSecrets()))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	data := dataField(body)
	require.NotContains(t, data, "password", "stream path must strip INPUT_ONLY")
	require.Equal(t, "public-name", data["name"])
}
