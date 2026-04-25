// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/data/audit"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// ActorExtractor extracts an Actor from an HTTP request.
type ActorExtractor func(r *http.Request) audit.Actor

type options struct {
	actorExtractor ActorExtractor
	ignorePaths    []string
	ignorePatterns []*regexp.Regexp
	ignoreMethods  []string
	logger         *slog.Logger
}
