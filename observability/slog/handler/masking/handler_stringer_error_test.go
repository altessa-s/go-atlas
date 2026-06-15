// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// errUnexportedMsg models the shape that triggers the regression: the
// human-readable payload lives in an unexported field and is reachable only
// through Error()/String() (as for a proto validation error).
type errUnexportedMsg struct{ msg string }

func (e *errUnexportedMsg) Error() string { return e.msg }

// TestHandler_KindAny_StringerOnlyErrorSurvivesDescent guards against the
// masking descent dropping a value whose payload is unexported. Since
// fmt.Stringer stopped being treated as atomic, such a value (e.g. a proto
// validation error) is descended into via reflection, yields no exported
// fields, and previously collapsed to an empty slog group — which slog omits,
// making the attribute disappear from the log entirely.
func TestHandler_KindAny_StringerOnlyErrorSurvivesDescent(t *testing.T) {
	inner, store := newCaptureHandler()
	// WithDefaults mirrors the service wiring (factory uses it), so the masking
	// reflect-descent path is active rather than the no-rules fast path.
	h := NewHandler(inner, WithDefaults())
	lg := slog.New(h)

	lg.LogAttrs(context.Background(), slog.LevelWarn, "validation failed",
		slog.Any("error", &errUnexportedMsg{msg: "slug: required field cannot be cleared"}))

	v, ok := store.attrs["error"]
	require.True(t, ok, "error attribute must reach the inner handler")
	require.Contains(t, v.String(), "required field cannot be cleared",
		"error message must survive masking descent, not collapse to an empty group")
}

// stringerOnly carries its payload in an unexported field, surfaced only via
// String() — the fmt.Stringer counterpart of errUnexportedMsg. Since ec3c53a8
// stopped treating fmt.Stringer as atomic, such a value is descended into and,
// having no exported fields, would collapse to an empty group without the
// walkStruct guard.
type stringerOnly struct{ v string }

func (s stringerOnly) String() string { return s.v }

// plainUnexported has neither String() nor Error() and no exported fields — it
// has no maskable substructure at all, so descent must keep it as-is rather
// than replace it with an empty group.
type plainUnexported struct{ x int }

// renderThroughMasking logs a single attribute through the masking handler into
// a real slog text handler (WithDefaults mirrors the service wiring). Unlike the
// capture handler, a real handler omits empty groups — which is exactly the slog
// behaviour that makes a collapsed attribute disappear from the log — so this is
// what actually proves an attribute survives to the output.
func renderThroughMasking(t *testing.T, attr slog.Attr) string {
	t.Helper()
	var buf bytes.Buffer
	h := NewHandler(slog.NewTextHandler(&buf, nil), WithDefaults())
	rec := slog.NewRecord(time.Time{}, slog.LevelWarn, "validation failed", 0)
	rec.AddAttrs(attr)
	require.NoError(t, h.Handle(context.Background(), rec))
	return buf.String()
}

// TestHandler_KindAny_NoExportedFieldsSurviveDescent generalises the
// protovalidator regression beyond errors: any KindAny value that reflect
// descent cannot turn into a NON-empty slog group (no exported fields, empty
// map, empty slice) must be kept as-is and rendered canonically — never
// collapsed into an empty group, which slog drops from the output entirely.
func TestHandler_KindAny_NoExportedFieldsSurviveDescent(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value any
		want  string // substring that must appear in the rendered line
	}{
		{"empty map", "meta", map[string]string{}, "meta"},
		{"empty slice", "ids", []string{}, "ids"},
		{"stringer only", "note", stringerOnly{v: "hi-stringer"}, "hi-stringer"},
		{"plain unexported struct", "blob", plainUnexported{x: 7}, "blob"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderThroughMasking(t, slog.Any(tc.key, tc.value))
			require.Contains(t, out, tc.key,
				"attribute key must survive masking descent, not collapse to an omitted empty group")
			require.Contains(t, out, tc.want)
		})
	}
}
