// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// FingerprintBuilder renders a query's fields into a deterministic
// fingerprint, so a consumer does not have to invent — and get right — its
// own stable encoding. Every value is length-prefixed, so two different
// field sets can never render to the same bytes, and fields contribute in
// the order they are written.
//
//	fp := keyset.NewFingerprintBuilder()
//	fp.Field("actor", q.ActorID)
//	fp.TimePtr("start", q.StartTime)
//	bindings.Filter = fp.Sum()
//
// The zero value is not usable; construct with [NewFingerprintBuilder].
type FingerprintBuilder struct {
	sb strings.Builder
}

// NewFingerprintBuilder returns an empty builder.
func NewFingerprintBuilder() *FingerprintBuilder {
	return &FingerprintBuilder{}
}

// Field appends one key/value pair.
func (b *FingerprintBuilder) Field(key, value string) *FingerprintBuilder {
	b.sb.WriteString(key)
	b.sb.WriteByte('=')
	b.sb.WriteString(strconv.Itoa(len(value)))
	b.sb.WriteByte(':')
	b.sb.WriteString(value)
	b.sb.WriteByte('\n')

	return b
}

// TimePtr appends a time bound, distinguishing "unset" from any value —
// nil and the zero time do not render alike.
func (b *FingerprintBuilder) TimePtr(key string, value *time.Time) *FingerprintBuilder {
	if value == nil {
		return b.Field(key, "")
	}

	return b.Field(key, strconv.FormatInt(value.UnixMilli(), 10))
}

// Sum returns the fingerprint of everything written so far. The builder is
// not reset: writing more fields and summing again yields the fingerprint
// of the longer rendering.
func (b *FingerprintBuilder) Sum() string {
	return Fingerprint([]byte(b.sb.String()))
}

// FingerprintJSON fingerprints an arbitrary filter value through its JSON
// encoding, which sorts map keys at every level — hashing Go's map
// iteration order directly would produce a different fingerprint on each
// call and reject every cursor.
//
// A value json cannot encode — NaN, a channel, a cycle — returns an error
// rather than a fingerprint, so two distinct bad filters never silently
// share one.
func FingerprintJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "encode filter for fingerprint")
	}

	return Fingerprint(encoded), nil
}
