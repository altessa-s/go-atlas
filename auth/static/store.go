// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"hash"
	"sync"
	"time"
)

// hmacKeySize is the byte length of the per-store HMAC key (matches SHA-256
// digest size, per NIST SP 800-107 §5.3.4).
const hmacKeySize = 32

// TokenStore validates a token and returns the data associated with it.
// Implementations must be safe for concurrent use.
type TokenStore interface {
	// Validate returns the data associated with token, or an error wrapping
	// [ErrTokenInvalid] / [ErrTokenEmpty] if the token is rejected.
	Validate(ctx context.Context, token string) (any, error)
}

// InMemoryStore is a [TokenStore] backed by an in-memory map keyed by the
// HMAC-SHA256 digest of each token. The plaintext token is never retained.
//
// Lookups are a single map probe — there is no linear scan or early-break
// loop. Timing therefore depends on token length only and not on token
// position or membership.
//
// The zero value is not usable; construct an [InMemoryStore] with
// [NewInMemoryStore].
type InMemoryStore struct {
	mu      sync.RWMutex
	tokens  map[string]any
	macPool sync.Pool
	metrics *Metrics
}

// NewInMemoryStore constructs an [InMemoryStore] with the supplied options.
//
// If [WithHMACKey] is not supplied, a 32-byte random key is generated for
// this instance using [crypto/rand]. The random key is not persisted —
// digests are valid only for the lifetime of the store.
func NewInMemoryStore(opt ...Option) *InMemoryStore {
	o := newOptions(opt...)
	if len(o.hmacKey) == 0 {
		o.hmacKey = randomKey(hmacKeySize)
	}

	s := &InMemoryStore{
		tokens:  make(map[string]any, len(o.initialTokens)),
		metrics: o.metrics,
		macPool: sync.Pool{
			New: func() any {
				return hmac.New(sha256.New, o.hmacKey)
			},
		},
	}
	for token, data := range o.initialTokens {
		s.tokens[s.digest(token)] = data
	}
	s.metrics.SetActiveTokens(len(s.tokens))
	return s
}

// Validate implements [TokenStore].
func (s *InMemoryStore) Validate(_ context.Context, token string) (any, error) {
	start := s.metricsStart()

	if token == "" {
		s.recordValidation(false, start)
		return nil, ErrTokenEmpty
	}

	key := s.digest(token)
	s.mu.RLock()
	data, ok := s.tokens[key]
	s.mu.RUnlock()

	s.recordValidation(ok, start)
	if !ok {
		return nil, ErrTokenInvalid
	}
	return data, nil
}

// AddToken inserts or updates a token in the store.
func (s *InMemoryStore) AddToken(token string, data any) {
	if token == "" {
		return
	}
	key := s.digest(token)
	s.mu.Lock()
	s.tokens[key] = data
	n := len(s.tokens)
	s.mu.Unlock()
	s.metrics.SetActiveTokens(n)
}

// RemoveToken removes a token from the store. It is a no-op when the token
// is not registered.
func (s *InMemoryStore) RemoveToken(token string) {
	if token == "" {
		return
	}
	key := s.digest(token)
	s.mu.Lock()
	delete(s.tokens, key)
	n := len(s.tokens)
	s.mu.Unlock()
	s.metrics.SetActiveTokens(n)
}

// TokenCount returns the number of tokens currently held by the store.
func (s *InMemoryStore) TokenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tokens)
}

// digest returns the HMAC-SHA256 digest of token, encoded as a raw byte
// string suitable for use as a map key.
func (s *InMemoryStore) digest(token string) string {
	mac, _ := s.macPool.Get().(hash.Hash)
	mac.Reset()
	mac.Write([]byte(token))
	sum := mac.Sum(nil)
	s.macPool.Put(mac)
	return string(sum)
}

func (s *InMemoryStore) metricsStart() time.Time {
	if s.metrics == nil {
		return time.Time{}
	}
	return time.Now()
}

func (s *InMemoryStore) recordValidation(success bool, start time.Time) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordValidation(success, time.Since(start))
}

// randomKey returns n bytes from crypto/rand. It panics on failure, which
// only happens when the system entropy source is unavailable — a state in
// which authentication cannot safely proceed.
func randomKey(n int) []byte {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic("auth/static: crypto/rand failed: " + err.Error())
	}
	return buf
}
