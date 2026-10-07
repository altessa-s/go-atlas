// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	"google.golang.org/protobuf/types/known/anypb"

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

// TestWrite_StripsInputOnly_Collections covers INPUT_ONLY fields reached
// through repeated and map message fields and a oneof, while OUTPUT_ONLY
// fields are left in the response.
func TestWrite_StripsInputOnly_Collections(t *testing.T) {
	t.Parallel()

	res := &testpb.Resource{
		Name:       "public-name",
		CreateTime: "2026-01-01T00:00:00Z",
		Aliases:    []*testpb.Profile{{DisplayName: "a", Secret: "alias-secret"}},
		Labels:     map[string]*testpb.Profile{"k": {DisplayName: "l", Secret: "label-secret"}},
		Source:     &testpb.Resource_SourceToken{SourceToken: "oneof-secret"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")
	require.NoError(t, writer.New().Write(rec, req, res))

	body := rec.Body.String()
	require.Contains(t, body, "2026-01-01T00:00:00Z", "OUTPUT_ONLY must survive response sanitization")
	require.Contains(t, body, "public-name")
	for _, secret := range []string{"alias-secret", "label-secret", "oneof-secret"} {
		require.NotContains(t, body, secret, "INPUT_ONLY in repeated, map and oneof fields must be stripped")
	}
}

// TestWrite_TypedNilMessage passes a nil *Resource: nothing to strip, no error.
func TestWrite_TypedNilMessage(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	require.NoError(t, writer.New().Write(rec, req, (*testpb.Resource)(nil)))
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestWrite_AnyIsNotSanitized pins the documented boundary: the contents of a
// google.protobuf.Any are opaque bytes and pass through unchanged.
func TestWrite_AnyIsNotSanitized(t *testing.T) {
	t.Parallel()

	packed, err := anypb.New(resourceWithSecrets())
	require.NoError(t, err)
	value := slices.Clone(packed.GetValue())

	data := dataField(writeJSON(t, writer.New(), packed))

	encoded, ok := data["value"].(string)
	require.True(t, ok)
	got, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	require.Equal(t, value, got, "Any contents are documented as not sanitized")
}

// TestWriteStream_SanitizationDisabled pins the opt-out on the streaming path.
func TestWriteStream_SanitizationDisabled(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	require.NoError(t, writer.New(writer.WithResponseSanitizationDisabled()).WriteStream(rec, req, resourceWithSecrets()))
	require.Contains(t, rec.Body.String(), "top-secret")
}

// TestWrite_SanitizationFailure exceeds the configured depth: the client gets
// a 500 without the payload, the caller gets ErrResponseSanitization wrapping
// the cause, and the failure is logged.
func TestWrite_SanitizationFailure(t *testing.T) {
	t.Parallel()

	for name, write := range map[string]func(*writer.Writer, http.ResponseWriter, *http.Request, any) error{
		"Write":       (*writer.Writer).Write,
		"WriteStream": (*writer.Writer).WriteStream,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer
			wr := writer.New(
				writer.WithResponseSanitizationMaxDepth(0),
				writer.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
			)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept", "application/json")

			err := write(wr, rec, req, resourceWithSecrets())
			require.ErrorIs(t, err, writer.ErrResponseSanitization)
			require.ErrorIs(t, err, fieldbehavior.ErrMaxDepthExceeded)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
			require.NotContains(t, rec.Body.String(), "top-secret")
			require.NotContains(t, rec.Body.String(), "public-name")
			require.Contains(t, logs.String(), "response sanitization failed")
		})
	}
}

// TestWrite_SanitizationMaxDepth shows a deep enough limit sanitizes normally.
func TestWrite_SanitizationMaxDepth(t *testing.T) {
	t.Parallel()

	data := dataField(writeJSON(t, writer.New(writer.WithResponseSanitizationMaxDepth(4)), resourceWithSecrets()))
	require.NotContains(t, data, "password")
}
