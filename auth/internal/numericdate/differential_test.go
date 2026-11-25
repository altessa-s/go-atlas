// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package numericdate

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDifferentialAgainstRat checks parsing, comparison and both roundings against exact rational arithmetic.
func TestDifferentialAgainstRat(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(1, 2))
	digits := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteByte(byte('0' + r.IntN(10)))
		}
		return b.String()
	}
	gen := func(base int64) string {
		var b strings.Builder
		if r.IntN(4) == 0 {
			b.WriteByte('-')
		}
		switch r.IntN(4) {
		case 0: // near base seconds, plain fraction
			fmt.Fprintf(&b, "%d", base+int64(r.IntN(5))-2)
			if r.IntN(2) == 0 {
				b.WriteString("." + digits(1+r.IntN(14)))
			}
		case 1: // exponent form: mantissa d.ddd e k
			m := digits(1 + r.IntN(22))
			m = strings.TrimLeft(m, "0")
			if m == "" {
				m = "0"
			}
			e := r.IntN(24) - 14
			if len(m) > 1 {
				m = m[:1] + "." + m[1:]
			}
			fmt.Fprintf(&b, "%se%d", m, e)
		case 2: // small numbers and zeros
			b.WriteString([]string{"0", "0.0", "0.000000000", "0.0000000001", "0.0000000009", "1", "1e0", "0e5", "0.5e-9", "5e-10", "1e-30"}[r.IntN(11)])
		default: // base with fraction near ns boundaries
			ns := []string{"000000000", "999999999", "000000001", "500000000"}[r.IntN(4)]
			tail := []string{"", "0", "1", "9", "5", "0000000001", "9999999999"}[r.IntN(7)]
			fmt.Fprintf(&b, "%d.%s%s", base, ns, tail)
		}
		return b.String()
	}
	const base = 1_750_000_000
	for i := range 50000 {
		s := gen(base)
		want, ok := new(big.Rat).SetString(s)
		if !ok {
			require.Failf(t, "differential mismatch", "oracle cannot parse %q", s)
		}
		d, ok := Parse(json.Number(s))
		lim := new(big.Rat).SetInt64(1 << 62)
		inRange := new(big.Rat).Abs(want).Cmp(lim) < 0
		if !ok {
			if inRange {
				require.Failf(t, "differential mismatch", "#%d Parse(%q) rejected an in-range value", i, s)
			}
			continue
		}
		neg := strings.HasPrefix(s, "-")
		nowSec := int64(base) + int64(r.IntN(5)) - 2
		if neg {
			nowSec = -nowSec
		}
		if r.IntN(6) == 0 {
			nowSec = int64(r.IntN(3)) - 1
		}
		nowNs := []int64{0, 1, 999999999, 500000000, int64(r.IntN(1e9))}[r.IntN(5)]
		now := time.Unix(nowSec, nowNs)
		off := []time.Duration{0, 1, -1, time.Second, -time.Second, 60 * time.Second, -60 * time.Second, time.Duration(r.Int64N(int64(4*time.Second))) - 2*time.Second}[r.IntN(8)]
		ref := new(big.Rat).SetInt64(now.Unix())
		ref.Add(ref, big.NewRat(int64(now.Nanosecond())+int64(off), 1e9))
		if got, exp := d.CompareOffset(FromTime(now), off), want.Cmp(ref); got != exp {
			require.Failf(t, "differential mismatch", "#%d %q vs now=%d.%09d off=%v: got %d want %d", i, s, now.Unix(), now.Nanosecond(), off, got, exp)
		}
		// Time rounds down, Expiry rounds up to the nanosecond.
		floor := new(big.Rat).SetInt64(d.Time().Unix())
		floor.Add(floor, big.NewRat(int64(d.Time().Nanosecond()), 1e9))
		ceil := new(big.Rat).SetInt64(d.Expiry().Unix())
		ceil.Add(ceil, big.NewRat(int64(d.Expiry().Nanosecond()), 1e9))
		step := big.NewRat(1, 1e9)
		if floor.Cmp(want) > 0 || new(big.Rat).Add(floor, step).Cmp(want) <= 0 {
			require.Failf(t, "differential mismatch", "#%d %q: Time()=%s is not the floor", i, s, floor.FloatString(12))
		}
		if ceil.Cmp(want) < 0 || new(big.Rat).Sub(ceil, step).Cmp(want) >= 0 {
			require.Failf(t, "differential mismatch", "#%d %q: Expiry()=%s is not the ceiling", i, s, ceil.FloatString(12))
		}
	}
}
