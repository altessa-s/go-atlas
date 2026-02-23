// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"context"
	"regexp"

	"github.com/altessa-s/go-atlas/data/audit"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// ActorExtractor extracts an Actor from a gRPC context.
type ActorExtractor func(ctx context.Context) audit.Actor

// RequestIDExtractor extracts the request ID from a context.
type RequestIDExtractor func(ctx context.Context) string

type options struct {
	actorExtractor     ActorExtractor
	requestIDExtractor RequestIDExtractor
	ignoreMethods      []string
	ignorePatterns     []*regexp.Regexp
}
