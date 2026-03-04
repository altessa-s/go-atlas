// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/auth/opa"
)

func TestNew_Valid(t *testing.T) {
	t.Parallel()

	// Cannot actually connect, but validation should pass.
	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if source == nil {
		t.Fatal("New() returned nil source")
	}
	defer source.Close()
}

func TestNew_MissingEndpoint(t *testing.T) {
	t.Parallel()

	_, err := New(
		WithToken("test-token"),
		WithProjectID(42),
	)
	if err == nil {
		t.Fatal("New() without endpoint should fail")
	}

	if !errors.Is(err, ErrEndpointRequired) {
		t.Errorf("New() error = %v, want ErrEndpointRequired", err)
	}
}

func TestNew_MissingToken(t *testing.T) {
	t.Parallel()

	_, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithProjectID(42),
	)
	if err == nil {
		t.Fatal("New() without token should fail")
	}

	if !errors.Is(err, ErrTokenRequired) {
		t.Errorf("New() error = %v, want ErrTokenRequired", err)
	}
}

func TestNew_MissingProjectID(t *testing.T) {
	t.Parallel()

	_, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
	)
	if err == nil {
		t.Fatal("New() without project ID should fail")
	}

	if !errors.Is(err, ErrProjectIDRequired) {
		t.Errorf("New() error = %v, want ErrProjectIDRequired", err)
	}
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
		WithRef("develop"),
		WithDir("policies"),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	expected := "gitlab:42/policies@develop"
	if name := source.Name(); name != expected {
		t.Errorf("Name() = %q, want %q", name, expected)
	}
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() after Close() should fail")
	}

	if !errors.Is(err, opa.ErrSourceClosed) {
		t.Errorf("Fetch() error = %v, want ErrSourceClosed", err)
	}
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("first Close() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("second Close() failed: %v", err)
	}
}

func TestSource_DefaultRef(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	expected := "gitlab:42/@main"
	if name := source.Name(); name != expected {
		t.Errorf("Name() = %q, want %q (default ref should be main)", name, expected)
	}
}
