// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/http/server/codec"
)

const (
	// DefaultCodecJSON is the default codec content type.
	DefaultCodecJSON = "application/json"

	// DefaultResponseSanitizationMaxDepth is the default nesting depth response
	// sanitization walks. See [WithResponseSanitizationMaxDepth].
	DefaultResponseSanitizationMaxDepth = fieldbehavior.DefaultMaxDepth

	// DefaultMaxBodySize is the default cap, in bytes, on a request body
	// [Writer.Read] accepts (4 MiB). See [WithMaxBodySize].
	DefaultMaxBodySize int64 = 4 << 20

	// UnlimitedBodySize passed to [WithMaxBodySize] disables the request body
	// cap. Any negative value has the same effect.
	UnlimitedBodySize int64 = -1
)

// options holds configuration for the Writer.
type options struct {
	registry                   *codec.Registry `optgen:"default=codec.DefaultRegistry()"`
	responseBuilder            Builder         `optgen:"default=NewDefault()"`
	defaultCodec               string          `optgen:"default=DefaultCodecJSON"`
	fallbackOnNegotiationError bool            `optgen:"default=true"`
	logger                     *slog.Logger    // for logging fallback errors
	errorConverter             ErrorConverter  // optional custom error converter

	// responseSanitizationDisabled turns off field_behavior INPUT_ONLY
	// stripping of proto.Message responses. Sanitization is on by default (zero
	// value false); set it with [WithResponseSanitizationDisabled] only when the
	// caller has an external reason to emit INPUT_ONLY fields on the read path.
	responseSanitizationDisabled bool

	// maxBodySize caps the request body [Writer.Read] accepts, in bytes. Zero
	// selects [DefaultMaxBodySize]; a negative value ([UnlimitedBodySize])
	// disables the cap.
	maxBodySize int64 `optgen:"default=DefaultMaxBodySize"`

	// responseSanitizationMaxDepth bounds how deep response sanitization walks
	// nested messages; a deeper response fails with ErrResponseSanitization.
	responseSanitizationMaxDepth int `optgen:"default=DefaultResponseSanitizationMaxDepth"`
}
