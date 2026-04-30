// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
)

// Regression test: slice fields declared inside `,inline` embedded structs
// must be loadable from environment variables using the array index syntax
// (e.g. PARENT__CHILD__SLICE__0). Before the fix to findFieldByPath, the
// secondary StructField lookup only walked direct fields of the parent,
// so embedded slice fields were silently dropped — leaving callers with
// the misleading impression that env-var arrays work generally when in
// fact they only worked at the top level.

type embeddedFilter struct {
	IgnoreMethods  []string `yaml:"ignoreMethods"`
	IgnorePatterns []string `yaml:"ignorePatterns"`
}

type embeddedEnable struct {
	Enabled bool `yaml:"enabled"`
}

type embeddedInterceptor struct {
	embeddedEnable `yaml:",inline"`
	embeddedFilter `yaml:",inline"`

	KeyHeader string `yaml:"keyHeader"`
}

type embeddedRoot struct {
	Direct      []string             `yaml:"direct"`
	Interceptor *embeddedInterceptor `yaml:"interceptor"`
}

func TestLoad_EnvSlice_InsideInlineEmbeddedStruct(t *testing.T) {
	t.Setenv("DIRECT__0", "top-a")
	t.Setenv("DIRECT__1", "top-b")
	t.Setenv("INTERCEPTOR__ENABLED", "true")
	t.Setenv("INTERCEPTOR__KEY_HEADER", "x-key")
	t.Setenv("INTERCEPTOR__IGNORE_METHODS__0", "/svc/A")
	t.Setenv("INTERCEPTOR__IGNORE_METHODS__1", "/svc/B")
	t.Setenv("INTERCEPTOR__IGNORE_PATTERNS__0", `.*/(get.*|list.*|exists)$`)

	cfg := &embeddedRoot{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)

	require.Equal(t, []string{"top-a", "top-b"}, cfg.Direct,
		"top-level slice should load (regression baseline)")
	require.NotNil(t, cfg.Interceptor)
	require.True(t, cfg.Interceptor.Enabled,
		"scalar inside embedded struct should load")
	require.Equal(t, "x-key", cfg.Interceptor.KeyHeader,
		"direct scalar at parent level should load")

	require.Equal(t, []string{"/svc/A", "/svc/B"}, cfg.Interceptor.IgnoreMethods,
		"slice inside embedded struct must load via env (was nil before fix)")
	require.Equal(t, []string{`.*/(get.*|list.*|exists)$`}, cfg.Interceptor.IgnorePatterns,
		"single-element slice inside embedded struct must load via env (was nil before fix)")
}

func TestLoad_EnvSlice_InsideInlineEmbeddedStruct_Cleanup(t *testing.T) {
	// Sanity: when no embedded slice envs are set, the field stays nil/empty
	// instead of being mis-populated by leakage from sibling envs.
	os.Unsetenv("INTERCEPTOR__IGNORE_METHODS__0")
	os.Unsetenv("INTERCEPTOR__IGNORE_PATTERNS__0")

	cfg := &embeddedRoot{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)

	if cfg.Interceptor != nil {
		require.Empty(t, cfg.Interceptor.IgnoreMethods)
		require.Empty(t, cfg.Interceptor.IgnorePatterns)
	}
}

// Regression test: trailing `$` in env values must be preserved by the
// env-var unwrapping pass in BOTH non-strict and strict modes. Previously
// the mixed-string scan loop silently stripped any `$` not followed by a
// variable name, corrupting regex patterns like `.*/get.*$`.

func TestLoad_EnvValue_PreservesTrailingDollar_NonStrict(t *testing.T) {
	t.Setenv("INTERCEPTOR__IGNORE_PATTERNS__0", `.*/(get.*|list.*|exists)$`)

	cfg := &embeddedRoot{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.NotNil(t, cfg.Interceptor)
	require.Equal(t, []string{`.*/(get.*|list.*|exists)$`}, cfg.Interceptor.IgnorePatterns,
		"trailing `$` must be preserved as a literal, not stripped")
}

type trailingDollarConfig struct {
	Pattern  string   `yaml:"pattern"`
	Patterns []string `yaml:"patterns"`
}

func TestLoad_EnvValue_PreservesTrailingDollar_Strict(t *testing.T) {
	// Use a flat config to isolate this regression from unrelated strict-mode
	// behavior around embedded struct assignment.
	t.Setenv("PATTERN", `.*/(get.*|list.*|exists)$`)
	t.Setenv("PATTERNS__0", `^foo.*$`)

	cfg := &trailingDollarConfig{}
	l := loader.New(nil, loader.WithStrict())

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, `.*/(get.*|list.*|exists)$`, cfg.Pattern,
		"strict mode must preserve trailing `$` in scalar")
	require.Equal(t, []string{`^foo.*$`}, cfg.Patterns,
		"strict mode must preserve trailing `$` in slice element")
}

// Tests for `$$` escape syntax: lets users write a literal `$` even when it
// would otherwise look like an env-var reference (e.g. `$$HOME` to mean the
// literal text `$HOME`, not the user's home directory).

func TestLoad_EnvValue_DollarDollarEscape_NonStrict(t *testing.T) {
	t.Setenv("LITERAL", "5$")
	t.Setenv("PATTERN", "$$") // raw double dollar → literal "$"

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "$", cfg.Pattern, "`$$` must collapse to a single literal `$`")
}

func TestLoad_EnvValue_DollarDollarEscape_PreventsExpansion_NonStrict(t *testing.T) {
	t.Setenv("HOME_OVERRIDE", "should-not-appear")
	// `$$HOME_OVERRIDE` must yield literal text `$HOME_OVERRIDE`, NOT expansion.
	t.Setenv("PATTERN", "prefix-$$HOME_OVERRIDE-suffix")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "prefix-$HOME_OVERRIDE-suffix", cfg.Pattern,
		"`$$NAME` must produce literal `$NAME`, the env var must NOT be expanded")
}

func TestLoad_EnvValue_DollarDollarEscape_MixedWithExpansion_NonStrict(t *testing.T) {
	t.Setenv("REAL_VAR", "expanded")
	t.Setenv("PATTERN", "$REAL_VAR/$$REAL_VAR") // first expands, second is literal

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "expanded/$REAL_VAR", cfg.Pattern,
		"first `$REAL_VAR` expands; `$$REAL_VAR` stays literal")
}

func TestLoad_EnvValue_DollarDollarEscape_TripleDollar_NonStrict(t *testing.T) {
	t.Setenv("REAL_VAR", "expanded")
	// `$$$REAL_VAR` — first two collapse to literal `$`, third begins expansion.
	t.Setenv("PATTERN", "$$$REAL_VAR")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "$expanded", cfg.Pattern,
		"`$$$VAR` should produce literal `$` followed by expansion of $VAR")
}

func TestLoad_EnvValue_ChainedExpansion_EnvValueHasDollarDollar(t *testing.T) {
	// Edge case: VAR1 expands to a value that itself contains `$$`.
	// The escape must be honored at the point where it lands in the result,
	// not silently passed through or re-expanded.
	t.Setenv("CHAIN_TARGET", "$$literal")
	t.Setenv("PATTERN", "$CHAIN_TARGET")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "$literal", cfg.Pattern,
		"chained expansion must apply $$ escape from substituted env value")
}

func TestLoad_EnvValue_ChainedExpansion_EscapesPreventChainedExpansion(t *testing.T) {
	// VAR1 → "$$VAR2", VAR2 → "expanded". Result should be literal "$VAR2",
	// not the expansion of VAR2, even though VAR2 is defined.
	t.Setenv("CHAIN_INNER", "expanded")
	t.Setenv("CHAIN_OUTER", "$$CHAIN_INNER")
	t.Setenv("PATTERN", "$CHAIN_OUTER")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "$CHAIN_INNER", cfg.Pattern,
		"escaped name from chained expansion must NOT be further expanded")
}

func TestLoad_EnvValue_UndefinedVarBeforeEscape_NonStrict(t *testing.T) {
	// Undefined var (stripped) followed by escape: both must work in one pass.
	t.Setenv("PATTERN", "pre-$UNDEFINED_VAR_XYZ-$$mid-end")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "pre--$mid-end", cfg.Pattern,
		"undefined var stripped, then $$ escape applied")
}

func TestLoad_EnvValue_DollarDollarAtEndOfValue(t *testing.T) {
	t.Setenv("PATTERN", "abc$$")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "abc$", cfg.Pattern, "$$ at end of value must produce trailing literal $")
}

func TestLoad_EnvValue_DollarDollarEscape_Strict(t *testing.T) {
	t.Setenv("PATTERN", "$$")
	t.Setenv("PATTERNS__0", "$$LITERAL")
	t.Setenv("PATTERNS__1", "a$$b")

	cfg := &trailingDollarConfig{}
	l := loader.New(nil, loader.WithStrict())

	_, err := l.Load(cfg)
	require.NoError(t, err, "strict mode must NOT error on $$ escape (no var lookup performed)")
	require.Equal(t, "$", cfg.Pattern)
	require.Equal(t, []string{"$LITERAL", "a$b"}, cfg.Patterns,
		"strict mode must apply $$ escape and skip env lookup for the escaped name")
}
