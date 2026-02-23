// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package http_test

import (
	"net/http"
	"testing"

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

	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	if !called {
		t.Error("RoundTripperFunc not called")
	}
	if resp.StatusCode != 200 {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}
}
