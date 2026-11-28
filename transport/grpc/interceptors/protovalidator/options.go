// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package protovalidator

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
)

// Use defaults package for optgen code generation.
var _ = defaults.IgnorePatterns

// options holds configuration for the protovalidator interceptor.
type options struct {
	// logRawErrors enables logging error text, panic values, or stacks that may contain secrets.
	logRawErrors bool `optval:"param"`

	// ignoreMethods is a list of method names to skip validation for.
	// Example: ["/grpc.health.v1.Health/Check"]
	ignoreMethods []string

	// ignorePatterns is a list of regex patterns for methods to skip validation for.
	// By default, includes gRPC reflection and health check patterns.
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`

	// fieldInfoNormalization rewrites the request's google.api.field_info formatted values (UUID4, IPv4, IPv6,
	// IP addresses) in their normalized form once the validator has accepted it. A value that does not match its
	// format fails the call with codes.InvalidArgument and a BadRequest detail naming its field.
	fieldInfoNormalization bool `opt:"FieldInfoNormalization"`

	// logger is the slog.Logger for debug/info/error logging.
	// If not set, a discard logger is used (no logging).
	logger *slog.Logger
}
