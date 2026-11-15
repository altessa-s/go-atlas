// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/audit"
)

//go:generate go tool optgen generate --type=options

// ActorExtractor extracts an Actor from an HTTP request.
type ActorExtractor func(r *http.Request) audit.Actor

// Length caps of the client-controlled strings stored in an audit event.
const (
	// DefaultMaxPathLength is the default cap, in bytes, of the stored URL path.
	DefaultMaxPathLength = 2048
	// DefaultMaxUserAgentLength is the default cap, in bytes, of the stored
	// User-Agent.
	DefaultMaxUserAgentLength = 512
)

// TruncationSuffix marks a stored value that was cut to its length cap; it
// counts toward the cap.
const TruncationSuffix = "..."

type options struct {
	actorExtractor ActorExtractor
	ignorePaths    []string
	ignorePatterns []*regexp.Regexp
	ignoreMethods  []string
	logger         *slog.Logger
	// maxPathLength caps, in bytes, the URL path stored as the event's
	// resource path. A longer path is truncated, never rejected. Values below
	// 1 are ignored.
	maxPathLength int `optcheck:"min=1" optgen:"default=DefaultMaxPathLength,onfail=skip"`
	// maxUserAgentLength caps, in bytes, the User-Agent stored with the
	// actor, whether it comes from the request or from the actor extractor. A
	// longer value is truncated, never rejected. Values below 1 are ignored.
	maxUserAgentLength int `optcheck:"min=1" optgen:"default=DefaultMaxUserAgentLength,onfail=skip"`
}
