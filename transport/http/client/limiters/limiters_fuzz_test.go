// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
	"net/http"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type fuzzLimiter struct{}

func (l *fuzzLimiter) Allow(_ context.Context, _ string) error { return nil }

func FuzzRoundTripper_RoundTrip(f *testing.F) {
	f.Add("http://example.com/test")
	f.Add("http://localhost:8080/api/v1")
	f.Add("https://api.test:443/")

	next := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := NewRoundTripper(next, &fuzzLimiter{})

	f.Fuzz(func(t *testing.T, rawURL string) {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return // Invalid URL, skip
		}
		_, _ = rt.RoundTrip(req)
	})
}
