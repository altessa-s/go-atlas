// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Internal test package: these tests access validateOptions and newOptions
// to exercise the validation layer directly, which lets the suite run on
// any platform without actually applying rlimits to the test process
// (a lowered hard limit would propagate to every subsequent test in the
// binary).
package rlimits

import (
	"errors"
	"testing"
)

func TestValidateOptions_AcceptsEmpty(t *testing.T) {
	if err := validateOptions(newOptions()); err != nil {
		t.Errorf("empty options: got %v, want nil", err)
	}
}

func TestValidateOptions_AcceptsPositiveValues(t *testing.T) {
	o := newOptions(
		WithMemoryBytes(512<<20),
		WithMaxOpenFiles(4096),
		WithMaxProcesses(256),
		WithMaxFileSizeBytes(1<<30),
		WithDisableCoreDumps(),
	)
	if err := validateOptions(o); err != nil {
		t.Errorf("positive values: got %v, want nil", err)
	}
}

func TestValidateOptions_AcceptsZero(t *testing.T) {
	// Zero means "leave kernel default in place" — valid.
	o := newOptions(
		WithMemoryBytes(0),
		WithMaxOpenFiles(0),
	)
	if err := validateOptions(o); err != nil {
		t.Errorf("zero values: got %v, want nil", err)
	}
}

func TestValidateOptions_RejectsNegative(t *testing.T) {
	cases := []struct {
		name string
		opts []Option
	}{
		{"negative memory", []Option{WithMemoryBytes(-1)}},
		{"negative open files", []Option{WithMaxOpenFiles(-1)}},
		{"negative processes", []Option{WithMaxProcesses(-1)}},
		{"negative file size", []Option{WithMaxFileSizeBytes(-1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptions(newOptions(tc.opts...))
			if err == nil {
				t.Fatalf("validateOptions: got nil, want error")
			}
			if !errors.Is(err, ErrInvalidOption) {
				t.Errorf("validateOptions: got %v, want wrap of ErrInvalidOption", err)
			}
		})
	}
}

// TestApply_ValidatesBeforePlatformDispatch ensures that a bad option
// surfaces as a validation error on every platform, regardless of
// platform support. This is the regression guard for the public API
// contract that validation happens before [apply] is called.
func TestApply_ValidatesBeforePlatformDispatch(t *testing.T) {
	err := Apply(WithMemoryBytes(-1))
	if err == nil {
		t.Fatalf("Apply: got nil, want validation error")
	}
	if !errors.Is(err, ErrInvalidOption) {
		t.Errorf("Apply: got %v, want wrap of ErrInvalidOption", err)
	}
	if errors.Is(err, ErrUnsupported) {
		t.Errorf("Apply: got ErrUnsupported, want ErrInvalidOption")
	}
	if errors.Is(err, ErrFailed) {
		t.Errorf("Apply: got ErrFailed, want ErrInvalidOption")
	}
}

// TestSentinels_AreDistinct guards against a future refactor that
// accidentally collapses the three sentinels.
func TestSentinels_AreDistinct(t *testing.T) {
	pairs := [][2]error{
		{ErrFailed, ErrUnsupported},
		{ErrFailed, ErrInvalidOption},
		{ErrUnsupported, ErrInvalidOption},
	}
	for _, p := range pairs {
		if errors.Is(p[0], p[1]) {
			t.Errorf("%v should not match %v", p[0], p[1])
		}
	}
}
