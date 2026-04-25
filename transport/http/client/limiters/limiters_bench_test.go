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

type noopLimiter struct{}

func (l *noopLimiter) Allow(_ context.Context, _ string) error { return nil }

func BenchmarkRoundTripper_RoundTrip(b *testing.B) {
	next := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := NewRoundTripper(next, &noopLimiter{})
	req, _ := http.NewRequest("GET", "http://example.com/test", nil)

	for b.Loop() {
		_, _ = rt.RoundTrip(req)
	}
}
