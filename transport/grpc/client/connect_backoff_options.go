// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"time"

	"google.golang.org/grpc/backoff"
)

// Default connection backoff, the values of grpc-go's [backoff.DefaultConfig].
const (
	// DefaultConnectBackoffBaseDelay is the delay after the first failed connection attempt.
	DefaultConnectBackoffBaseDelay = time.Second

	// DefaultConnectBackoffMultiplier is the factor the delay grows by after each further failed attempt.
	DefaultConnectBackoffMultiplier = 1.6

	// DefaultConnectBackoffJitter is the fraction by which each delay is randomized.
	DefaultConnectBackoffJitter = 0.2

	// DefaultConnectBackoffMaxDelay caps the delay between connection attempts.
	DefaultConnectBackoffMaxDelay = 120 * time.Second
)

// connectBackoffOptions controls the backoff between attempts to connect to
// one address, which grpc-go applies per subchannel. All fields are populated
// by generated [ConnectBackoffOption] setters in connect_backoff_options_gen.go.
type connectBackoffOptions struct {
	// baseDelay is the delay after the first failed connection attempt.
	baseDelay time.Duration `optgen:"default=DefaultConnectBackoffBaseDelay"`

	// multiplier is the factor the delay grows by after each further failed attempt.
	multiplier float64 `optgen:"default=DefaultConnectBackoffMultiplier" optval:"positive"`

	// jitter is the fraction by which each delay is randomized; 0 disables jitter.
	jitter float64 `optcheck:"min=0" optgen:"default=DefaultConnectBackoffJitter,onfail=skip"`

	// maxDelay caps the delay between connection attempts.
	maxDelay time.Duration `optgen:"default=DefaultConnectBackoffMaxDelay"`
}

// config returns the options as a grpc-go backoff configuration; nil options
// mean [backoff.DefaultConfig].
func (o *connectBackoffOptions) config() backoff.Config {
	if o == nil {
		return backoff.DefaultConfig
	}
	return backoff.Config{BaseDelay: o.baseDelay, Multiplier: o.multiplier, Jitter: o.jitter, MaxDelay: o.maxDelay}
}
