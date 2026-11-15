// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/audit"
)

//go:generate go tool optgen generate --type=options

// ActorExtractor extracts an Actor from a gRPC context.
type ActorExtractor func(ctx context.Context) audit.Actor

// Length caps of the client-controlled strings stored in an audit event.
const (
	// DefaultMaxMethodLength is the default cap, in bytes, of the stored
	// service and full method names.
	DefaultMaxMethodLength = 1024
	// DefaultMaxUserAgentLength is the default cap, in bytes, of the stored
	// User-Agent.
	DefaultMaxUserAgentLength = 512
	// DefaultMaxMessageLength is the default cap, in bytes, of the stored gRPC
	// status message.
	DefaultMaxMessageLength = 1024
)

// TruncationSuffix marks a stored value that was cut to its length cap; it
// counts toward the cap.
const TruncationSuffix = "..."

type options struct {
	actorExtractor ActorExtractor
	ignoreMethods  []string
	ignorePatterns []*regexp.Regexp
	logger         *slog.Logger
	// maxMethodLength caps, in bytes, the service and full method names stored
	// as the event's resource. They come from the request path, which a client
	// controls when the server routes unknown services to a handler. A longer
	// name is truncated, never rejected. Values below 1 are ignored.
	maxMethodLength int `optcheck:"min=1" optgen:"default=DefaultMaxMethodLength,onfail=skip"`
	// maxUserAgentLength caps, in bytes, the User-Agent the actor extractor
	// stores with the actor. A longer value is truncated, never rejected.
	// Values below 1 are ignored.
	maxUserAgentLength int `optcheck:"min=1" optgen:"default=DefaultMaxUserAgentLength,onfail=skip"`
	// maxMessageLength caps, in bytes, the gRPC status message stored as the
	// result message. Handlers commonly echo request values in it. A longer
	// message is truncated, never rejected. Values below 1 are ignored.
	maxMessageLength int `optcheck:"min=1" optgen:"default=DefaultMaxMessageLength,onfail=skip"`
}
