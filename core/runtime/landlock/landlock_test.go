// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Internal test package: these tests access validateOptions and newOptions
// to exercise the validation layer directly, which lets the suite run on
// any platform without actually applying Landlock to the test process
// (which would be irreversible and break every subsequent test in the
// same binary).
package landlock

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateOptions_AcceptsEmpty(t *testing.T) {
	// An empty options set is valid — the caller may want a total
	// filesystem lockout.
	require.NoError(t, validateOptions(newOptions()))
}

func TestValidateOptions_AcceptsAbsolutePaths(t *testing.T) {
	o := newOptions(
		WithReadPaths("/etc", "/usr/lib", "/lib64"),
		WithReadWritePaths("/var/lib/myservice", "/var/log/myservice"),
	)
	require.NoError(t, validateOptions(o))
}

func TestValidateOptions_RejectsRelativePath(t *testing.T) {
	cases := []struct {
		name string
		opts []Option
	}{
		{
			name: "dot-slash in read paths",
			opts: []Option{WithReadPaths("/etc", "./relative")},
		},
		{
			name: "bare name in read paths",
			opts: []Option{WithReadPaths("etc")},
		},
		{
			name: "parent-relative in read-write paths",
			opts: []Option{WithReadWritePaths("../data")},
		},
		{
			name: "empty string in read paths",
			opts: []Option{WithReadPaths("")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptions(newOptions(tc.opts...))
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidOption)
		})
	}
}

// TestApply_ValidatesBeforePlatformDispatch ensures that a bad option
// surfaces as a validation error on every platform, regardless of kernel
// support. This is the regression guard for the public API contract that
// validation happens before [apply] is called.
func TestApply_ValidatesBeforePlatformDispatch(t *testing.T) {
	err := Apply(WithReadPaths("./relative"))
	require.Error(t, err)
	// Validation errors wrap ErrInvalidOption exclusively; they must not
	// look like ErrUnsupported or ErrFailed, so callers can fan out on
	// "bad config" vs "kernel rejected a well-formed ruleset".
	require.ErrorIs(t, err, ErrInvalidOption)
	require.NotErrorIs(t, err, ErrUnsupported)
	require.NotErrorIs(t, err, ErrFailed)
}

// TestWithReadPaths_NilAndEmptyAreNoop verifies the optgen-generated
// behavior: empty variadic calls leave the options untouched.
func TestWithReadPaths_NilAndEmptyAreNoop(t *testing.T) {
	cases := []struct {
		name string
		opt  Option
	}{
		{"no args", WithReadPaths()},
		{"nil slice", WithReadPaths(nil...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newOptions(tc.opt)
			require.Empty(t, o.readPaths)
		})
	}
}

// TestWithReadPaths_Accumulates verifies that multiple calls to
// WithReadPaths append rather than replace.
func TestWithReadPaths_Accumulates(t *testing.T) {
	o := newOptions(
		WithReadPaths("/etc"),
		WithReadPaths("/usr/lib"),
	)
	require.Equal(t, []string{"/etc", "/usr/lib"}, o.readPaths)
}
