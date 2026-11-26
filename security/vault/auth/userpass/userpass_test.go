// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package userpass_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/userpass"

	vaultApi "github.com/hashicorp/vault/api"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"basic", "admin", "password123"},
		{"with spaces", "  admin  ", "  pass  "},
		{"empty values", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := userpass.New(tt.username, tt.password)
			require.NotNil(t, m)
			require.Equal(t, "userpass", m.Name())
		})
	}
}

func TestNew_WithMountPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mountPath string
	}{
		{"string mount path", "/auth/userpass"},
		{"custom path", "/custom/userpass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := userpass.New("admin", "pass", auth.WithMountPathOption[*auth.MountPathMethod](tt.mountPath))
			require.NotNil(t, m)
			require.Equal(t, tt.mountPath, m.MountPath())
		})
	}
}

func TestNew_WithMountPathPointer(t *testing.T) {
	t.Parallel()

	path := "/auth/custom-userpass"
	m := userpass.New("admin", "pass", auth.WithMountPathOption[*auth.MountPathMethod](&path))
	require.NotNil(t, m)
	require.Equal(t, path, m.MountPath())
}

func TestAuthMethod_Shutdown(t *testing.T) {
	t.Parallel()

	m := userpass.New("admin", "pass")
	require.NoError(t, m.Shutdown())
}

func TestAuthenticate_PreservesPasswordSpaces(t *testing.T) {
	t.Parallel()

	const rawPassword = "  leading and trailing spaces  "
	var (
		gotPath     string
		gotPassword string
	)
	transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload map[string]string
		require.NoError(t, json.Unmarshal(body, &payload))
		gotPassword = payload["password"]
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"auth":{"client_token":"issued"}}`)),
			Request:    req,
		}, nil
	})
	client, err := vaultApi.NewClient(&vaultApi.Config{
		Address:    "http://vault.test",
		MaxRetries: 0,
		HttpClient: &http.Client{Transport: transport},
	})
	require.NoError(t, err)

	m := userpass.New("  alice  ", rawPassword)
	secret, err := m.Authenticate(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, "issued", secret.Auth.ClientToken)
	require.Equal(t, "/v1/auth/userpass/login/alice", gotPath)
	require.Equal(t, rawPassword, gotPassword)
}
