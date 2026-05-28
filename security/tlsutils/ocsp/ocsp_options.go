// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"net/http"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// FailureMode controls how [Stapler] reacts when it cannot fetch a
// valid OCSP response for a certificate. The default (Soft) preserves
// the historical behavior — return the cert without a staple so the
// TLS handshake still succeeds. Hard mode fails the handshake instead,
// which is appropriate when the operator wants strict revocation
// enforcement: a revoked certificate whose responder is unreachable
// should NOT be served.
type FailureMode string

const (
	// FailureModeSoft (default) returns the certificate without an OCSP
	// staple when the responder is unreachable or returns an invalid
	// response. Maximizes availability — a transient OCSP outage does
	// not break new connections — but a revoked certificate may be
	// served alongside its valid TLS material.
	FailureModeSoft FailureMode = "soft"

	// FailureModeHard returns an error from the GetCertificate /
	// GetClientCertificate callbacks when an OCSP staple cannot be
	// produced, aborting the TLS handshake. Use this in environments
	// where the cost of a temporary outage is acceptable but the cost
	// of serving a possibly-revoked certificate is not (PCI-DSS,
	// regulated industries, security-sensitive APIs).
	FailureModeHard FailureMode = "hard"

	// DefaultFailureMode is the production-safe-for-availability
	// default — Soft. Operators MUST explicitly opt in to Hard via
	// [WithFailureMode] because hard-fail breaks fresh connections
	// whenever the OCSP responder is down.
	DefaultFailureMode = FailureModeSoft
)

// DefaultMaxCacheEntries caps the in-memory OCSP-response cache. The
// existing lazy-cleanup pass only runs every defaultCleanupInterval
// calls and only removes EXPIRED entries — so an mTLS server presenting
// many distinct client certificates (or being asked to staple for many
// chains) grows the cache without bound between cleanup ticks. 4096 is
// enough to absorb realistic workloads while keeping the cap honest.
const DefaultMaxCacheEntries = 4096

type options struct {
	httpClient        *http.Client
	retryPolicy       RetryPolicy `optgen:"notnil"`
	logger            *slog.Logger
	enableCompression bool        `opt:"Compression"`
	failureMode       FailureMode `optgen:"manual,default=DefaultFailureMode"`
	maxCacheEntries   int         `optgen:"default=DefaultMaxCacheEntries"`

	// Scheduler configuration
	scheduler       corescheduler.TaskRegistrar `optgen:"notnil"`
	refreshSchedule string
}

// WithFailureMode selects how the [Stapler] reacts when it cannot
// produce a valid OCSP staple. Unknown / empty modes leave the default
// ([DefaultFailureMode]) in place — mirrors the pattern used by the
// rest of the repo (plugins.SignatureMode, oidc.JWKSFailureMode,
// config.TLSSkipVerifyMode).
func WithFailureMode(mode FailureMode) Option {
	return func(o *options) {
		switch mode {
		case FailureModeSoft, FailureModeHard:
			o.failureMode = mode
		}
	}
}
