// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package regexanchor

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestEndOfText(t *testing.T) {
	t.Parallel()

	tests := []struct{ pattern, want string }{
		{`abc`, `abc`},
		{`abc$`, `abc\z`},
		{`^$`, `^\z`},
		{`^A.*e$`, `^A.*e\z`},
		{`a$$`, `a\z\z`},
		{`\$`, `\$`},
		{`\\$`, `\\\z`},
		{`[$]`, `[$]`},
		{`[]$]`, `[]$]`},
		{`[^]$]$`, `[^]$]\z`},
		{`[[:alpha:]$]$`, `[[:alpha:]$]\z`},
		{`[\]$]$`, `[\]$]\z`},
		{`\Q$\E$`, `\Q$\E\z`},
		{`\Qa$`, `\Qa$`},
		{`(?m)a$`, `(?m)a$`},
		{`(?m:a$)b$`, `(?m:a$)b\z`},
		{`(?m)a$(?-m)b$`, `(?m)a$(?-m)b\z`},
		{`(?m)(a$(?-m)b$)c$`, `(?m)(a$(?-m)b\z)c$`},
		{`(a$)|b$`, `(a\z)|b\z`},
		{`(?i)a$`, `(?i)a\z`},
		{`(?im-s:a$)`, `(?im-s:a$)`},
		{`(?P<n>x$)`, `(?P<n>x\z)`},
		{`(?<n>x$)`, `(?<n>x\z)`},
		{`(?:x$)`, `(?:x\z)`},
		{`\x{24}$`, `\x{24}\z`},
		{`é$`, `é\z`},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			t.Parallel()
			got, err := EndOfText(tt.pattern)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestEndOfTextKeepsRE2Semantics checks that the rewrite matches exactly
// what the original matches under RE2, on inputs built around newlines.
func TestEndOfTextKeepsRE2Semantics(t *testing.T) {
	t.Parallel()

	patterns := []string{
		`abc$`, `^$`, `^abc$`, `(?m)^abc$`, `(?m:c$)|^a`, `a\nb$`, `[$c]$`, `(?s).$`, `.*$`, `\Q$\E`, `(?m)c$(?-m)\n$`,
	}
	inputs := []string{"", "\n", "abc", "abc\n", "x\nabc", "abc\nx", "a\nb", "a\nb\n", "$", "c$", "abc\n\n"}

	for _, p := range patterns {
		got, err := EndOfText(p)
		require.NoError(t, err, p)
		orig, anchored := regexp.MustCompile(p), regexp.MustCompile(got)
		for _, in := range inputs {
			require.Equal(t, orig.MatchString(in), anchored.MatchString(in), "pattern %q → %q on %q", p, got, in)
		}
	}
}

func TestEndOfTextRejects(t *testing.T) {
	t.Parallel()

	_, err := EndOfText(`(a$`)
	require.ErrorIs(t, err, filter.ErrInvalidRegex)

	// The self-check refuses a rewrite that changes the meaning.
	require.ErrorIs(t, verify(`a$`, `a$`), filter.ErrInvalidRegex, "the WasDollar marker must not survive")
	require.ErrorIs(t, verify(`a$`, `b\z`), filter.ErrInvalidRegex)
	require.ErrorIs(t, verify(`a$`, `a(\z`), filter.ErrInvalidRegex)
	require.NoError(t, verify(`a$`, `a\z`))
}
