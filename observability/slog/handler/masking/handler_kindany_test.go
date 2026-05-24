// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// findMaskedLeaf walks an arbitrary slog.Value and reports the first
// leaf attribute whose key matches `name`. Returns the leaf as its
// final string representation. Used by the tests below to peek inside
// the slog.GroupValue trees produced by the recursive masker without
// reproducing the slog handler chain.
func findMaskedLeaf(v slog.Value, name string) (string, bool) {
	if v.Kind() != slog.KindGroup {
		return "", false
	}
	for _, a := range v.Group() {
		if a.Key == name {
			return a.Value.String(), true
		}
		if got, ok := findMaskedLeaf(a.Value, name); ok {
			return got, true
		}
	}
	return "", false
}

type leakyPayload struct {
	UserID   string
	Password string
	Token    string
}

// TestHandler_KindAny_MasksNestedStructFields is the primary regression
// guard for the audit finding. A struct passed through slog.Any with a
// non-matching top-level key MUST have its sensitive subfields
// (Password, Token) masked. Before the fix, fmt.Sprint flattened the
// entire struct into a string and downstream handlers wrote the secrets
// verbatim.
func TestHandler_KindAny_MasksNestedStructFields(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithField("Password", FullMask()),
		WithField("Token", FullMask()),
	)

	payload := leakyPayload{
		UserID:   "u-123",
		Password: "p@ssw0rd",
		Token:    "Bearer xyz",
	}

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.Any("payload", payload)) // key "payload" is NOT in the mask list

	require.NoError(t, h.Handle(t.Context(), rec))

	got, ok := store.attrs["payload"]
	require.True(t, ok, "top-level attr must still be emitted")

	pw, ok := findMaskedLeaf(got, "Password")
	require.True(t, ok, "Password leaf must be reachable inside the masked group")
	require.Equal(t, "********", pw, "nested Password must be masked, not leaked as fmt.Sprint output")

	tok, ok := findMaskedLeaf(got, "Token")
	require.True(t, ok)
	require.Equal(t, "********", tok)

	uid, ok := findMaskedLeaf(got, "UserID")
	require.True(t, ok, "non-sensitive field must be preserved")
	require.Equal(t, "u-123", uid, "UserID must NOT be masked — only configured fields are touched")
}

// TestHandler_KindAny_MasksMapEntries covers the map[string]any case
// (common shape for opaque request payloads).
func TestHandler_KindAny_MasksMapEntries(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithField("apiKey", FullMask()),
	)

	payload := map[string]any{
		"user":   "alice",
		"apiKey": "sk-1234567890",
	}

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.Any("request", payload))

	require.NoError(t, h.Handle(t.Context(), rec))

	got := store.attrs["request"]

	masked, ok := findMaskedLeaf(got, "apiKey")
	require.True(t, ok, "apiKey must be reachable inside the masked group")
	require.Equal(t, "********", masked)

	user, ok := findMaskedLeaf(got, "user")
	require.True(t, ok)
	require.Equal(t, "alice", user)
}

// TestHandler_KindAny_MasksSliceOfStructs proves descent works through
// slice elements — a common shape for batch payloads.
func TestHandler_KindAny_MasksSliceOfStructs(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithField("Password", FullMask()),
	)

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.Any("users", []leakyPayload{
		{UserID: "a", Password: "secret-a"},
		{UserID: "b", Password: "secret-b"},
	}))

	require.NoError(t, h.Handle(t.Context(), rec))

	got := store.attrs["users"]

	// Slice descent emits each element as `[N]` with a nested group;
	// both Password leaves must be masked.
	require.Equal(t, slog.KindGroup, got.Kind(), "slice descent must produce a group")

	pw, ok := findMaskedLeaf(got, "Password")
	require.True(t, ok, "at least one Password must be reachable")
	require.Equal(t, "********", pw)
}

// TestHandler_KindAny_RespectsJSONTagName confirms the masker uses the
// json tag (when present) for field name matching, aligning with what
// JSON handlers typically emit so configured masks behave consistently.
func TestHandler_KindAny_RespectsJSONTagName(t *testing.T) {
	type Body struct {
		ClientToken string `json:"token"`
	}
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithField("token", FullMask()), // matches json tag, not Go field name
	)

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.Any("body", Body{ClientToken: "secret"}))

	require.NoError(t, h.Handle(t.Context(), rec))

	v := store.attrs["body"]
	masked, ok := findMaskedLeaf(v, "token")
	require.True(t, ok, "field must be renamed to its json tag during descent")
	require.Equal(t, "********", masked)
}

// TestHandler_KindAny_SkipsAtomicTypes pins the safety guarantee that
// values with their own canonical representation (time.Time, etc.) are
// NOT flattened into a slog group — they pass through as-is so
// downstream handlers render them natively.
func TestHandler_KindAny_SkipsAtomicTypes(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithField("Password", FullMask()),
	)

	when := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.Any("ts", when))

	require.NoError(t, h.Handle(t.Context(), rec))

	got := store.attrs["ts"]
	require.NotEqual(t, slog.KindGroup, got.Kind(),
		"time.Time must NOT be flattened into a slog group — it has its own canonical representation")
}

// TestHandler_KindAny_NilValueIsHarmless covers a nil any value — the
// walker must not panic.
func TestHandler_KindAny_NilValueIsHarmless(t *testing.T) {
	inner, _ := newCaptureHandler()
	h := NewHandler(inner,
		WithField("Password", FullMask()),
	)

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.Any("nope", any(nil)))

	require.NoError(t, h.Handle(t.Context(), rec))
}
