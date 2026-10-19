// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrNotVersion is returned by [Tag.Version] for a tag that [FromVersion]
// did not produce: a strong tag, the zero tag, or a value that is not a
// canonical non-negative decimal int64.
var ErrNotVersion = errors.New("etag: not a version tag")

// Errors of [ParseVersion] and [CheckVersion].
var (
	// ErrMissing reports an empty etag where one is required.
	ErrMissing = errors.New("etag: missing")

	// ErrMalformed reports an etag that is not one [FromVersion] issues.
	ErrMalformed = errors.New("etag: not an issued version etag")

	// ErrMismatch reports an etag issued for another version of the
	// resource: it changed since the client read it. It wraps
	// core/errors.ErrConflict, the optimistic-concurrency sentinel.
	ErrMismatch = fmt.Errorf("etag: %w", coreerrs.ErrConflict)
)

// FromVersion returns the weak entity-tag of a stored resource version,
// W/"<version>" in canonical decimal — the etag of a resource guarded by an
// integer version (optimistic concurrency). It is weak because
// equal versions mean semantically, not byte-for-byte, equal
// representations. A negative version is not a version and yields the zero
// (absent) Tag.
func FromVersion(version int64) Tag {
	if version < 0 {
		return Tag{}
	}
	return Tag{value: strconv.FormatInt(version, 10), weak: true}
}

// Version returns the version a [FromVersion] tag carries. It fails with
// [ErrNotVersion] unless t is weak and its value is the canonical decimal of
// a non-negative int64: no sign, no leading zeros (except "0" itself), no
// other characters, no overflow — so exactly one tag maps to each version.
func (t Tag) Version() (int64, error) {
	if !t.weak || !canonicalDecimal(t.value) {
		return 0, ErrNotVersion
	}
	v, err := strconv.ParseInt(t.value, 10, 64)
	if err != nil {
		return 0, ErrNotVersion
	}
	return v, nil
}

// canonicalDecimal reports whether s is "0" or a run of decimal digits
// without a leading zero.
func canonicalDecimal(s string) bool {
	if s == "" || (s[0] == '0' && len(s) > 1) {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ParseVersion returns the resource version of an etag a client sends back
// with a mutation, accepting exactly the form [FromVersion] issues:
// W/"<version>" with a canonical decimal version and nothing around it. An
// empty s is [ErrMissing]. A bare number, a strong tag, a non-canonical,
// non-numeric or negative version, and MaxInt64 (the next version would
// overflow) are [ErrMalformed]. W/"0" is accepted only [WithSingleton], for a
// singleton resource that has not been saved yet: every other stored version
// is positive.
func ParseVersion(s string, opts ...VersionOption) (int64, error) {
	if s == "" {
		return 0, ErrMissing
	}
	o := newVersionOptions(opts...)
	tag, err := Parse(s)
	if err != nil || tag.String() != s {
		// Malformed, or not the exact rendering (surrounding whitespace).
		return 0, ErrMalformed
	}
	v, err := tag.Version()
	if err != nil || v == math.MaxInt64 || (v == 0 && !o.singleton) {
		return 0, ErrMalformed
	}
	return v, nil
}

// CheckVersion validates the etag of a mutation against the resource's
// current version: it fails like [ParseVersion] for a missing or malformed
// etag, and with [ErrMismatch] when the etag was issued for another version.
// Use it when the caller holds the current version; a store that compares
// versions itself (compare-and-swap) takes the version from [ParseVersion]
// and reports its own conflict.
func CheckVersion(s string, current int64, opts ...VersionOption) error {
	v, err := ParseVersion(s, opts...)
	if err != nil {
		return err
	}
	if v != current {
		return ErrMismatch
	}
	return nil
}
