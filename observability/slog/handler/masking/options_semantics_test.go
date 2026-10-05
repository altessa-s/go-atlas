// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

// logJSON logs a single record with attrs through a masking handler built
// from opts and returns the decoded JSON record.
func logJSON(t *testing.T, opts []masking.Option, attrs ...any) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(masking.NewHandler(slog.NewJSONHandler(&buf, nil), opts...))
	logger.Info("test", attrs...)

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	return record
}

// TestDefaultMask_AppliesToNilMaskRules pins the semantics of WithDefaultMask:
// it is the mask for rules registered without a MaskFunc. Before the fix the
// option was stored but never read, and a nil-mask rule leaked its value.
func TestDefaultMask_AppliesToNilMaskRules(t *testing.T) {
	t.Parallel()

	fixed := masking.FixedMask("[X]")

	tests := []struct {
		name string
		opts []masking.Option
		key  string
		want string
	}{
		{
			name: "field with nil mask uses WithDefaultMask",
			opts: []masking.Option{masking.WithDefaultMask(fixed), masking.WithField("secret", nil)},
			key:  "secret",
			want: "[X]",
		},
		{
			name: "pattern with nil mask uses WithDefaultMask",
			opts: []masking.Option{masking.WithDefaultMask(fixed), masking.WithPattern(`(?i)^ssn$`, nil)},
			key:  "SSN",
			want: "[X]",
		},
		{
			name: "nil mask without WithDefaultMask falls back to FullMask",
			opts: []masking.Option{masking.WithField("secret", nil)},
			key:  "secret",
			want: masking.FullMask()("value-123"),
		},
		{
			name: "WithDefaults keeps an earlier WithDefaultMask",
			opts: []masking.Option{masking.WithDefaultMask(fixed), masking.WithDefaults(), masking.WithField("pin", nil)},
			key:  "pin",
			want: "[X]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			record := logJSON(t, tc.opts, tc.key, "value-123")
			require.Equal(t, tc.want, record[tc.key])
		})
	}
}

// TestDefaultMask_DoesNotMaskUnmatchedFields guards the other half of the
// contract: the default mask is not a catch-all for fields no rule names.
func TestDefaultMask_DoesNotMaskUnmatchedFields(t *testing.T) {
	t.Parallel()

	record := logJSON(t,
		[]masking.Option{masking.WithDefaultMask(masking.FixedMask("[X]")), masking.WithField("secret", nil)},
		"user", "alice",
	)
	require.Equal(t, "alice", record["user"])
}

// TestWithDefaults_StaysCaseInsensitive is the regression for WithDefaults
// silently switching exact field matching to case-sensitive: mixed-case keys
// that only the exact-name defaults cover (no default pattern matches them)
// leaked verbatim.
func TestWithDefaults_StaysCaseInsensitive(t *testing.T) {
	t.Parallel()

	const (
		bearer = "Bearer abcdefghijklmnop"
		email  = "someone@example.com"
	)

	record := logJSON(t, []masking.Option{masking.WithDefaults()},
		"Authorization", bearer,
		"EMAIL", email,
	)
	require.NotEqual(t, bearer, record["Authorization"])
	require.NotEqual(t, email, record["EMAIL"])
}

// TestWithDefaults_CaseSensitiveStillOptIn checks that an explicit
// WithCaseSensitive is honored together with WithDefaults, in either order.
func TestWithDefaults_CaseSensitiveStillOptIn(t *testing.T) {
	t.Parallel()

	orders := map[string][]masking.Option{
		"defaults first":       {masking.WithDefaults(), masking.WithCaseSensitive()},
		"case-sensitive first": {masking.WithCaseSensitive(), masking.WithDefaults()},
	}

	for name, opts := range orders {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			record := logJSON(t, opts, "EMAIL", "someone@example.com", "email", "someone@example.com")
			require.Equal(t, "someone@example.com", record["EMAIL"], "case-sensitive matching must not fold EMAIL")
			require.NotEqual(t, "someone@example.com", record["email"])
		})
	}
}

// TestCaseFoldCollision_LastRegistrationWins pins deterministic precedence
// for keys that differ only by case. Folding used to range over the fields
// map, so an explicit "Email" rule could randomly lose to the default
// "email" mask. Repetition defeats a lucky map iteration order.
func TestCaseFoldCollision_LastRegistrationWins(t *testing.T) {
	t.Parallel()

	const value = "someone@example.com"
	fixed := masking.FixedMask("[X]")

	for range 50 {
		record := logJSON(t,
			[]masking.Option{masking.WithDefaults(), masking.WithField("Email", fixed)},
			"email", value, "Email", value,
		)
		require.Equal(t, "[X]", record["email"])
		require.Equal(t, "[X]", record["Email"])

		record = logJSON(t,
			[]masking.Option{masking.WithField("Email", fixed), masking.WithDefaults()},
			"Email", value,
		)
		require.Equal(t, masking.EmailMask()(value), record["Email"], "the later WithDefaults registration wins")
	}
}

// TestNew_ReportsInvalidPatterns is the regression for invalid WithPattern
// rules being dropped without trace: New reports each one, wrapping
// ErrInvalidPattern, and returns no handler.
func TestNew_ReportsInvalidPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
	}{
		{name: "does not compile", pattern: "(unclosed"},
		{name: "empty", pattern: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, err := masking.New(slog.DiscardHandler,
				masking.WithPattern(`(?i)token`, masking.FullMask()),
				masking.WithPattern(tc.pattern, masking.FullMask()),
			)
			require.ErrorIs(t, err, masking.ErrInvalidPattern)
			require.Nil(t, h)
		})
	}
}

// TestNew_ReportsEveryInvalidPattern checks that all invalid patterns are
// reported, not just the first.
func TestNew_ReportsEveryInvalidPattern(t *testing.T) {
	t.Parallel()

	_, err := masking.New(slog.DiscardHandler,
		masking.WithPattern("(a", masking.FullMask()),
		masking.WithPattern("[b", masking.FullMask()),
	)
	require.ErrorIs(t, err, masking.ErrInvalidPattern)
	require.ErrorContains(t, err, `"(a"`)
	require.ErrorContains(t, err, `"[b"`)
}

// TestNew_ValidOptionsMasks is the positive control: valid options build a
// handler that masks like NewHandler.
func TestNew_ValidOptionsMasks(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h, err := masking.New(slog.NewJSONHandler(&buf, nil),
		masking.WithPattern(`(?i)token`, masking.FixedMask("[T]")),
	)
	require.NoError(t, err)
	slog.New(h).Info("x", "AccessToken", "abc")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	require.Equal(t, "[T]", record["AccessToken"])
}

// TestNewHandler_SkipsInvalidPatterns pins NewHandler's documented contract:
// an invalid pattern is skipped while the valid rules keep working.
func TestNewHandler_SkipsInvalidPatterns(t *testing.T) {
	t.Parallel()

	record := logJSON(t,
		[]masking.Option{
			masking.WithPattern("(unclosed", masking.FullMask()),
			masking.WithPattern(`(?i)token`, masking.FixedMask("[T]")),
		},
		"AccessToken", "abc", "user", "alice",
	)
	require.Equal(t, "[T]", record["AccessToken"])
	require.Equal(t, "alice", record["user"])
}
