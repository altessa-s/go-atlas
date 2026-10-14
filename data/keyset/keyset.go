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
	"hash"
	"strings"
	"sync"
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
	key      *signer
	previous []*signer
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
	previous := make([]*signer, len(o.previousKeys))
	for i, k := range o.previousKeys {
		if len(k) < MinKeyLength {
			return nil, fmt.Errorf("%w: previous key of %d bytes, minimum %d", ErrWeakKey, len(k), MinKeyLength)
		}
		previous[i] = newSigner(k)
	}
	return &Codec{key: newSigner(key), previous: previous, opts: o}, nil
}

// Issue returns a token carrying payload, bound to b.
func (c *Codec) Issue(payload []byte, b Bindings) (string, error) {
	if len(payload) > MaxPayloadLength {
		return "", fmt.Errorf("%w: %d bytes, maximum %d", ErrPayloadTooLarge, len(payload), MaxPayloadLength)
	}

	buf := rawBufPool.Get().(*[maxRawLength]byte) //nolint:errcheck // pool holds only this type
	defer rawBufPool.Put(buf)

	body := buf[:0]
	body = append(body, version)
	body = binary.BigEndian.AppendUint64(body, uint64(c.opts.clock().UnixMilli())) //nolint:gosec // epoch milliseconds are positive
	body = appendDigests(body, b)
	body = binary.AppendUvarint(body, uint64(len(payload)))
	body = append(body, payload...)
	body = c.key.appendMAC(body, body)

	return encodeToken(body), nil
}

// Resolve verifies token and returns its payload. The signature is checked
// first; only then are expiry and the bindings compared, so a forged token
// never reveals which binding it would have failed.
func (c *Codec) Resolve(token string, b Bindings) ([]byte, error) {
	if token == "" || len(token) > maxTokenLength || !canonicalAlphabet(token) {
		return nil, ErrInvalidToken
	}
	buf := rawBufPool.Get().(*[maxRawLength]byte) //nolint:errcheck // pool holds only this type
	defer rawBufPool.Put(buf)

	rawLength, err := decodeToken(buf[:], token)
	if err != nil || rawLength < headerLength+1+macLength {
		return nil, ErrInvalidToken
	}
	raw := buf[:rawLength]

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
	sortDigest, filterDigest, subjectDigest := digest(b.Sort), digest(b.Filter), digest(b.Subject)
	switch {
	case !hmac.Equal(digests[:digestLength], sortDigest[:]):
		return nil, ErrSortChanged
	case !hmac.Equal(digests[digestLength:2*digestLength], filterDigest[:]):
		return nil, ErrFilterChanged
	case !hmac.Equal(digests[2*digestLength:], subjectDigest[:]):
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
	if c.key.verify(body, mac) {
		return true
	}
	for _, k := range c.previous {
		if k.verify(body, mac) {
			return true
		}
	}
	return false
}

// rawBufPool holds scratch buffers for a decoded or not yet encoded token.
// A token is not secret (the client holds it), so buffers are not zeroed.
var rawBufPool = sync.Pool{New: func() any { return new([maxRawLength]byte) }}

// signer signs with one immutable key. HMAC states are pooled; each is
// checked out by a single caller and Reset before use.
type signer struct {
	pool sync.Pool
}

type macState struct {
	h   hash.Hash
	sum [macLength]byte
}

func newSigner(key []byte) *signer {
	// The key is copied so later changes to the caller's buffer cannot alter
	// which tokens verify.
	key = bytes.Clone(key)
	return &signer{pool: sync.Pool{New: func() any {
		return &macState{h: hmac.New(sha256.New, key)}
	}}}
}

// appendMAC appends the MAC of body to dst.
func (s *signer) appendMAC(dst, body []byte) []byte {
	st := s.pool.Get().(*macState) //nolint:errcheck // pool holds only this type
	defer s.pool.Put(st)
	st.h.Reset()
	st.h.Write(body)
	return append(dst, st.h.Sum(st.sum[:0])...)
}

// verify reports in constant time whether mac is the MAC of body.
func (s *signer) verify(body, mac []byte) bool {
	st := s.pool.Get().(*macState) //nolint:errcheck // pool holds only this type
	defer s.pool.Put(st)
	st.h.Reset()
	st.h.Write(body)
	return hmac.Equal(mac, st.h.Sum(st.sum[:0]))
}

// codecChunk is the number of encoded characters handled per step by
// encodeToken and decodeToken. It is a multiple of 4, so every chunk but the
// last holds whole base64 quanta and chunked coding equals one-shot coding.
const (
	codecChunk = 256
	// rawChunk is the number of raw bytes encoded into one codecChunk.
	rawChunk = codecChunk / 4 * 3
)

// encodeToken returns the encoding of raw, building the string in place.
func encodeToken(raw []byte) string {
	var sb strings.Builder
	sb.Grow(tokenEncoding.EncodedLen(len(raw)))
	var chunk [codecChunk]byte
	for len(raw) > 0 {
		n := min(len(raw), rawChunk)
		tokenEncoding.Encode(chunk[:], raw[:n])
		sb.Write(chunk[:tokenEncoding.EncodedLen(n)])
		raw = raw[n:]
	}
	return sb.String()
}

// decodeToken decodes token into dst without converting it to a byte slice.
// dst must hold the decoded length.
func decodeToken(dst []byte, token string) (int, error) {
	var chunk [codecChunk]byte
	n := 0
	for len(token) > 0 {
		k := copy(chunk[:], token)
		m, err := tokenEncoding.Decode(dst[n:], chunk[:k])
		if err != nil {
			return 0, err
		}
		n += m
		token = token[k:]
	}
	return n, nil
}

func appendDigests(dst []byte, b Bindings) []byte {
	sortDigest, filterDigest, subjectDigest := digest(b.Sort), digest(b.Filter), digest(b.Subject)
	dst = append(dst, sortDigest[:]...)
	dst = append(dst, filterDigest[:]...)
	return append(dst, subjectDigest[:]...)
}

// digest returns a truncated SHA-256 of a binding.
func digest(s string) [digestLength]byte {
	sum := sha256.Sum256([]byte(s))
	return [digestLength]byte(sum[:digestLength])
}
