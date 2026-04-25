// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package http_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	corehttp "github.com/altessa-s/go-atlas/core/net/http"
)

func TestRoundTripperFunc(t *testing.T) {
	called := false
	fn := corehttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200}, nil
	})

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	resp, err := fn.RoundTrip(req)

	require.NoError(t, err, "RoundTrip failed")
	require.True(t, called, "RoundTripperFunc not called")
	require.Equal(t, 200, resp.StatusCode)
}
