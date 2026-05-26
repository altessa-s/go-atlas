// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"log/slog"
	"time"
)

// DefaultTimeout applies when [WithTimeout] is not used or set to a
// non-positive value.
const DefaultTimeout = 30 * time.Second

// options holds the configuration applied by [Option] values.
type options struct {
	apiKey  string
	timeout time.Duration
	logger  *slog.Logger
}

// Option configures a [Client] at construction time.
type Option func(*options)

// WithAPIKey sets the Meilisearch master or admin API key. An empty key
// disables the Authorization header — useful for local development against
// an open instance.
func WithAPIKey(key string) Option {
	return func(o *options) { o.apiKey = key }
}

// WithTimeout overrides the HTTP client timeout. Non-positive values fall
// back to [DefaultTimeout].
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithLogger sets the structured logger used for client events. When
// omitted, a discard logger is used.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

// newOptions applies opts on top of defaults and clamps invalid values.
func newOptions(opts []Option) options {
	o := options{
		timeout: DefaultTimeout,
		logger:  slog.New(slog.DiscardHandler),
	}
	for _, apply := range opts {
		apply(&o)
	}
	if o.timeout <= 0 {
		o.timeout = DefaultTimeout
	}
	return o
}
