// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_MissingEndpoint(t *testing.T) {
	t.Parallel()

	_, err := New(
		WithToken("test-token"),
		WithProjectID(42),
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrEndpointRequired), "New() error = %v, want ErrEndpointRequired", err)
}

func TestNew_MissingToken(t *testing.T) {
	t.Parallel()

	_, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithProjectID(42),
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTokenRequired), "New() error = %v, want ErrTokenRequired", err)
}

func TestNew_MissingProjectID(t *testing.T) {
	t.Parallel()

	_, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrProjectIDRequired), "New() error = %v, want ErrProjectIDRequired", err)
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
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, "gitlab:42/policies@develop", source.Name())
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	require.NoError(t, err)

	require.NoError(t, source.Close())

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrSourceClosed), "Fetch() error = %v, want ErrSourceClosed", err)
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	require.NoError(t, err)

	require.NoError(t, source.Close(), "first Close() failed")
	require.NoError(t, source.Close(), "second Close() failed")
}

func TestSource_DefaultRef(t *testing.T) {
	t.Parallel()

	source, err := New(
		WithEndpoint("https://gitlab.example.com"),
		WithToken("test-token"),
		WithProjectID(42),
	)
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, "gitlab:42/@main", source.Name(), "default ref should be main")
}
