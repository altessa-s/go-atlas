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
}
