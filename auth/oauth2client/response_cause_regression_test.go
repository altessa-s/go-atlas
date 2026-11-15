// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreio "github.com/altessa-s/go-atlas-core/io"
)

func TestFormResponseReadCausePreserved(t *testing.T) {
	t.Parallel()
	const secret = "private-read-secret"
	cause := errors.New(secret)
	client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Request: req,
			Body: io.NopCloser(coreio.NewErrorReader(cause)),
		}, nil
	})}
	var logs bytes.Buffer
	err := requestBoundaryError(t, "form", "https://idp.example/token", client, &logs)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
	require.NotContains(t, err.Error(), secret)
	require.NotEmpty(t, logs.String())
	require.NotContains(t, logs.String(), secret)
}

func TestFormResponseJSONCausePreserved(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		check      func(*testing.T, error)
	}{
		{"syntax", `{"access_token":"private-json-secret",`, func(t *testing.T, err error) {
			t.Helper()
			var cause *json.SyntaxError
			require.ErrorAs(t, err, &cause)
		}},
		{"type", `{"access_token":{"private-json-secret":true}}`, func(t *testing.T, err error) {
			t.Helper()
			var cause *json.UnmarshalTypeError
			require.ErrorAs(t, err, &cause)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK, Request: req,
					Body: io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			})}
			var logs bytes.Buffer
			err := requestBoundaryError(t, "form", "https://idp.example/token", client, &logs)
			require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
			tc.check(t, err)
			require.NotContains(t, err.Error(), "private-json-secret")
			require.NotEmpty(t, logs.String())
			require.NotContains(t, logs.String(), "private-json-secret")
		})
	}
}
