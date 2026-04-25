// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/data/audit"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// ActorExtractor extracts an Actor from a gRPC context.
type ActorExtractor func(ctx context.Context) audit.Actor

type options struct {
	actorExtractor ActorExtractor
	ignoreMethods  []string
	ignorePatterns []*regexp.Regexp
	logger         *slog.Logger
}
