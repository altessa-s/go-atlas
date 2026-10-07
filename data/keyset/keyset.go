// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Errors returned by [Codec.Resolve] and [New].
var (
	// ErrInvalidToken is returned for a malformed token or a bad signature.
	ErrInvalidToken = errors.New("keyset: invalid page token")
	// ErrExpiredToken is returned for a token older than the configured TTL.
	ErrExpiredToken = errors.New("keyset: page token expired")
	// ErrSortChanged is returned when the query's sort differs from the one
	// the token was issued for.
	ErrSortChanged = errors.New("keyset: sort changed since the page token was issued")
	// ErrFilterChanged is returned when the query's filter differs from the
	// one the token was issued for. Resuming under another filter could reach
	// data the original query excluded, so it is rejected.
	ErrFilterChanged = errors.New("keyset: filter changed since the page token was issued")
	// ErrSubjectMismatch is returned when a token issued to one principal is
	// presented by another.
	ErrSubjectMismatch = errors.New("keyset: page token issued to another subject")
	// ErrPayloadTooLarge is returned by [Codec.Issue] for a payload longer
	// than [MaxPayloadLength].
	ErrPayloadTooLarge = errors.New("keyset: payload too large")
	// ErrWeakKey is returned by [New] for a key shorter than [MinKeyLength].
	ErrWeakKey = errors.New("keyset: signing key too short")
)

// Bindings tie a token to the query it was issued for. Each value is an
// opaque fingerprint chosen by the caller; [Codec.Resolve] rejects a token
// whose bindings differ.
type Bindings struct {
	// Sort fingerprints the ordering.
	Sort string
	// Filter fingerprints the predicate, excluding the page position and size.
	Filter string
	// Subject identifies the principal the token was issued to.
	Subject string
}

const (
	version      byte = 1
	digestLength      = 16 // truncated SHA-256 of a binding
	macLength         = sha256.Size
	// headerLength covers version, issue time and the three binding digests.
	headerLength = 1 + 8 + 3*digestLength
	// maxRawLength bounds the decoded token.
	maxRawLength = headerLength + binary.MaxVarintLen64 + MaxPayloadLength + macLength
)

// maxTokenLength bounds the encoded token accepted by Resolve.
var maxTokenLength = tokenEncoding.EncodedLen(maxRawLength)

// tokenEncoding is strict so that every token has exactly one encoding: the
// unused bits of a final character must be zero.
var tokenEncoding = base64.RawURLEncoding.Strict()

// Codec issues and resolves signed, stateless page tokens. A token carries a
// position payload, its issue time and digests of its [Bindings], signed with
// HMAC-SHA256. It is safe for concurrent use.
type Codec struct {
	key      []byte
	previous [][]byte
	opts     *options
}

// New returns a Codec signing with key, which must be at least
// [MinKeyLength] bytes and the same on every replica that resolves the
// tokens.
func New(key []byte, opts ...Option) (*Codec, error) {
	if len(key) < MinKeyLength {
		return nil, fmt.Errorf("%w: %d bytes, minimum %d", ErrWeakKey, len(key), MinKeyLength)
	}
	o := newOptions(opts...)
	// Keys are copied so later changes to the caller's buffers cannot alter
	// which tokens verify.
	previous := make([][]byte, len(o.previousKeys))
	for i, k := range o.previousKeys {
		if len(k) < MinKeyLength {
			return nil, fmt.Errorf("%w: previous key of %d bytes, minimum %d", ErrWeakKey, len(k), MinKeyLength)
		}
		previous[i] = bytes.Clone(k)
	}
	return &Codec{key: bytes.Clone(key), previous: previous, opts: o}, nil
}

// Issue returns a token carrying payload, bound to b.
func (c *Codec) Issue(payload []byte, b Bindings) (string, error) {
	if len(payload) > MaxPayloadLength {
		return "", fmt.Errorf("%w: %d bytes, maximum %d", ErrPayloadTooLarge, len(payload), MaxPayloadLength)
	}

	body := make([]byte, 0, headerLength+binary.MaxVarintLen64+len(payload)+macLength)
	body = append(body, version)
	body = binary.BigEndian.AppendUint64(body, uint64(c.opts.clock().UnixMilli())) //nolint:gosec // epoch milliseconds are positive
	body = appendDigests(body, b)
	body = binary.AppendUvarint(body, uint64(len(payload)))
	body = append(body, payload...)
	body = append(body, sign(c.key, body)...)

	return tokenEncoding.EncodeToString(body), nil
}

// Resolve verifies token and returns its payload. The signature is checked
// first; only then are expiry and the bindings compared, so a forged token
// never reveals which binding it would have failed.
func (c *Codec) Resolve(token string, b Bindings) ([]byte, error) {
	if token == "" || len(token) > maxTokenLength || !canonicalAlphabet(token) {
		return nil, ErrInvalidToken
	}
	raw, err := tokenEncoding.DecodeString(token)
	if err != nil || len(raw) < headerLength+1+macLength {
		return nil, ErrInvalidToken
	}

	body, mac := raw[:len(raw)-macLength], raw[len(raw)-macLength:]
	if !c.verify(body, mac) {
		return nil, ErrInvalidToken
	}
	if body[0] != version {
		return nil, ErrInvalidToken
	}

	issued := time.UnixMilli(int64(binary.BigEndian.Uint64(body[1:9]))) //nolint:gosec // signed by us
	now := c.opts.clock()
	if issued.After(now.Add(c.opts.maxClockSkew)) {
		return nil, ErrInvalidToken
	}
	if !c.opts.expiryDisabled && now.Sub(issued) > c.opts.ttl {
		return nil, ErrExpiredToken
	}

	digests := body[9:headerLength]
	switch {
	case !hmac.Equal(digests[:digestLength], digest(b.Sort)):
		return nil, ErrSortChanged
	case !hmac.Equal(digests[digestLength:2*digestLength], digest(b.Filter)):
		return nil, ErrFilterChanged
	case !hmac.Equal(digests[2*digestLength:], digest(b.Subject)):
		return nil, ErrSubjectMismatch
	}

	n, size := binary.Uvarint(body[headerLength:])
	rest := body[headerLength+max(size, 0):]
	if size <= 0 || n > MaxPayloadLength || n != uint64(len(rest)) {
		return nil, ErrInvalidToken
	}

	return bytes.Clone(rest), nil
}

// canonicalAlphabet reports whether token uses only the URL-safe base64
// alphabet. The decoder skips CR and LF, which would otherwise let several
// strings decode to the same token.
func canonicalAlphabet(token string) bool {
	for i := range len(token) {
		switch c := token[i]; {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// verify reports whether mac signs body under the current or a previous key.
func (c *Codec) verify(body, mac []byte) bool {
	if hmac.Equal(mac, sign(c.key, body)) {
		return true
	}
	for _, k := range c.previous {
		if hmac.Equal(mac, sign(k, body)) {
			return true
		}
	}
	return false
}

func sign(key, body []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(body)
	return h.Sum(nil)
}

func appendDigests(dst []byte, b Bindings) []byte {
	dst = append(dst, digest(b.Sort)...)
	dst = append(dst, digest(b.Filter)...)
	return append(dst, digest(b.Subject)...)
}

// digest returns a truncated SHA-256 of a binding.
func digest(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:digestLength]
}
