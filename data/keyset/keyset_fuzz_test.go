// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/keyset"
)

// FuzzCodec_Resolve feeds arbitrary tokens to Resolve: it must never panic
// and must reject everything it did not issue.
func FuzzCodec_Resolve(f *testing.F) {
	// A fixed clock makes the issued token identical in every fuzz worker.
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, err := keyset.New(key, keyset.WithClock(func() time.Time { return at }))
	if err != nil {
		f.Fatal(err)
	}
	valid, _ := c.Issue([]byte("p"), bindings)
	f.Add(valid)
	f.Add("")
	f.Add("AAAA")

	f.Fuzz(func(t *testing.T, token string) {
		payload, err := c.Resolve(token, bindings)
		if err == nil {
			if token != valid || string(payload) != "p" {
				t.Fatalf("accepted a token it did not issue: %q", token)
			}
			return
		}
		if !errors.Is(err, keyset.ErrInvalidToken) && !errors.Is(err, keyset.ErrExpiredToken) &&
			!errors.Is(err, keyset.ErrSortChanged) && !errors.Is(err, keyset.ErrFilterChanged) &&
			!errors.Is(err, keyset.ErrSubjectMismatch) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
