// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/core/time/timeformat"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	"google.golang.org/protobuf/proto"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// PayloadRedactor transforms a request or response proto into a loggable value
// with sensitive fields removed. Return a redacted copy or a summary string;
// never the original credential-bearing fields.
type PayloadRedactor func(proto.Message) any

// options configures the logger interceptor.
type options struct {
	// timeFormat specifies how to format timestamps (default: TimeFormatRFC3339)
	timeFormat timeformat.Format `optgen:"default=timeformat.RFC3339"`
	// logResponse specifies whether to log response payloads
	logResponse bool
	// logRequest specifies whether to log request payloads
	logRequest bool
	// ignoreMethods contains methods to skip logging for
	ignoreMethods []string
	// ignorePatterns contains regex patterns to skip logging for
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`
	// logGrpcResponseCodes specifies gRPC response codes to always log
	logGrpcResponseCodes []uint32
	// ignoreGrpcResponseCodes specifies gRPC response codes to never log
	// takes precedence over logGrpcResponseCodes
	ignoreGrpcResponseCodes []uint32
	// contextLogger enables storing enriched logger in context.
	// Handlers can retrieve it via slogx.FromContextOrDefault(ctx).
	// If nil, context injection is disabled (default behavior).
	contextLogger *slog.Logger `optgen:"default=nil"`
	// payloadRedactor, when set, transforms a request or response proto into a
	// loggable value with sensitive fields removed before it is logged. When nil
	// the raw proto is logged. It only applies when logRequest/logResponse is on,
	// and is the only way to keep INPUT_ONLY/credential fields out of payload logs.
	payloadRedactor PayloadRedactor `optgen:"default=nil"`
}
