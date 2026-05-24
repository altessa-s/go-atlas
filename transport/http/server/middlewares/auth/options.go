// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"regexp"

	_ "github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
)

// defaultTokenExtractor is the bearer-token extractor applied when callers do
// not supply [WithTokenExtractor]. Declared at package scope so generated
// [defaultOptions] can reference it.
var defaultTokenExtractor = ExtractBearerToken()

// options holds configuration for the auth middleware.
type options struct {
	authFunc       AuthFunc
	tokenExtractor TokenExtractor `optgen:"default=defaultTokenExtractor"`
	errorHandler   ErrorHandler
	logger         *slog.Logger
	ignorePaths    []string
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`
}
