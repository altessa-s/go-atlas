// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/transport/internal/headers"
)

// DefaultRetryAfterMax caps how long a server's Retry-After may stall a retry
// of the client's retry layer (see [WithRetryAfterMax]).
const DefaultRetryAfterMax = time.Minute

// ParseRetryAfter reads the Retry-After header of h (RFC 9110 §10.2.3): a
// non-negative number of seconds or an HTTP date, relative to now. A date in
// the past yields zero. A positive maxDelay caps the result; a non-positive
// one leaves it uncapped. ok is false when the header is absent or malformed
// (including a negative number), in which case the duration is zero.
func ParseRetryAfter(h http.Header, now time.Time, maxDelay time.Duration) (d time.Duration, ok bool) {
	v := strings.TrimSpace(h.Get(headers.HeaderRetryAfter))
	if v == "" {
		return 0, false
	}
	if isDigits(v) {
		secs, err := strconv.ParseInt(v, 10, 64)
		if err != nil || secs > int64(math.MaxInt64/time.Second) {
			d = time.Duration(math.MaxInt64) // out of range: as long as allowed
		} else {
			d = time.Duration(secs) * time.Second
		}
	} else {
		at, err := http.ParseTime(v)
		if err != nil {
			return 0, false
		}
		d = max(at.Sub(now), 0)
	}
	if maxDelay > 0 {
		d = min(d, maxDelay)
	}
	return d, true
}

// isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// DefaultRetryAfterStatuses returns the reply statuses whose Retry-After the
// retry layer honors by default: 429 Too Many Requests and 503 Service
// Unavailable (see [WithRetryAfterStatuses]).
func DefaultRetryAfterStatuses() []int {
	return []int{http.StatusTooManyRequests, http.StatusServiceUnavailable}
}

// honorsRetryAfter reports whether the Retry-After of a reply with status is
// honored under statuses.
func honorsRetryAfter(statuses []int, status int) bool {
	return slices.Contains(statuses, status)
}
