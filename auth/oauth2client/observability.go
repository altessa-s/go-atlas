// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"log/slog"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// measure wraps a self-refreshing [oauth2.TokenSource] so each real token
// acquisition is recorded to metrics and logged. It returns inner unchanged when
// neither a metrics sink nor a logger is configured, keeping the default path
// allocation-free. Cache hits (the reuse source returning an unexpired token)
// are not recorded — only calls that produce a new access token or fail.
func measure(inner oauth2.TokenSource, grant string, m *Metrics, logger *slog.Logger) oauth2.TokenSource {
	if m == nil && logger == nil {
		return inner
	}
	return &measuredSource{inner: inner, grant: grant, metrics: m, logger: logger}
}

// measuredSource instruments an [oauth2.TokenSource]. It records a fetch only
// when the returned access token differs from the last one it saw, so the
// reuse source's cache hits do not inflate the counters.
type measuredSource struct {
	inner   oauth2.TokenSource
	grant   string
	metrics *Metrics
	logger  *slog.Logger

	mu   sync.Mutex
	last string // last observed access token
}

// Token returns a token from the wrapped source, recording latency and outcome
// of genuine acquisitions.
func (s *measuredSource) Token() (*oauth2.Token, error) {
	start := time.Now()
	tok, err := s.inner.Token()
	elapsed := time.Since(start)

	if err != nil {
		s.metrics.recordFetch(s.grant, false, elapsed)
		if s.logger != nil {
			s.logger.Warn("oauth2client: token fetch failed", "grant", s.grant, "err", err)
		}
		return nil, err
	}

	s.mu.Lock()
	fetched := tok.AccessToken != s.last
	s.last = tok.AccessToken
	s.mu.Unlock()

	if fetched {
		s.metrics.recordFetch(s.grant, true, elapsed)
	}
	return tok, nil
}
