// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testdbname

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// suffixBytes is the entropy of the random name suffix (8 hex digits).
const suffixBytes = 4

// New returns a throwaway database name: the prefix, the creation time as 8 hex digits of Unix seconds, an underscore and 8 random
// hex digits.
func New(prefix string, now time.Time) string {
	suffix := make([]byte, suffixBytes)
	_, _ = rand.Read(suffix)
	return fmt.Sprintf("%s%08x_%s", prefix, now.Unix(), hex.EncodeToString(suffix))
}

// Pattern returns the expression matching exactly the names [New] produces for prefix, capturing the creation time.
func Pattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile("^" + regexp.QuoteMeta(prefix) + `([0-9a-f]{8})_[0-9a-f]{8}$`)
}

// Stale reports whether name matches pattern and was created before cutoff. A name without a parsable creation time is never stale.
func Stale(pattern *regexp.Regexp, name string, cutoff time.Time) bool {
	m := pattern.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	secs, err := strconv.ParseInt(m[1], 16, 64)
	return err == nil && time.Unix(secs, 0).Before(cutoff)
}
