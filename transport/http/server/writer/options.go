// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/http/server/codec"
)

const (
	// DefaultCodecJSON is the default codec content type.
	DefaultCodecJSON = "application/json"
)

// options holds configuration for the Writer.
type options struct {
	registry                   *codec.Registry `optgen:"default=codec.DefaultRegistry()"`
	responseBuilder            Builder         `optgen:"default=NewDefault()"`
	defaultCodec               string          `optgen:"default=DefaultCodecJSON"`
	fallbackOnNegotiationError bool            `optgen:"default=true"`
	maxBodySize                int64           // 0 means no limit
	logger                     *slog.Logger    // for logging fallback errors
	errorConverter             ErrorConverter  // optional custom error converter

	// responseSanitizationDisabled turns off field_behavior INPUT_ONLY
	// stripping of proto.Message responses. Sanitization is on by default (zero
	// value false); set it with [WithResponseSanitizationDisabled] only when the
	// caller has an external reason to emit INPUT_ONLY fields on the read path.
	responseSanitizationDisabled bool

	// responseSanitizationMaxDepth bounds how deep response sanitization walks
	// nested messages; a deeper response fails with ErrResponseSanitization.
	responseSanitizationMaxDepth int `optgen:"default=fieldbehavior.DefaultMaxDepth"`
}
