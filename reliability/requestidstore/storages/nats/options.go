// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go tool optgen generate

import (
	"log/slog"
	"time"
)

// Default settings for the dedicated request ID bucket.
const (
	DefaultBucket        = "requestid"
	DefaultReplicas      = 1
	DefaultPurgeInterval = time.Minute
)

// DefaultCleanupGrace retains expired entries for an extra hour before physical cleanup.
const DefaultCleanupGrace = time.Hour

type options struct {
	// WithCleanupGrace sets the retention margin for clock skew. Values below one millisecond are ignored.
	cleanupGrace time.Duration `optgen:"default=DefaultCleanupGrace,onfail=skip" optcheck:"min=time.Millisecond"`
	bucket       string        `optgen:"default=DefaultBucket"`
	replicas     int           `optgen:"default=DefaultReplicas"`
	// WithPurgeInterval sets the time between cleanup passes. Zero disables the loop.
	purgeInterval time.Duration `optgen:"default=DefaultPurgeInterval,onfail=skip" optcheck:"min=0"`
	logger        *slog.Logger  `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}
