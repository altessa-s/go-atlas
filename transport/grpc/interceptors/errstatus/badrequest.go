// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"strings"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// BadRequest returns a codes.InvalidArgument status for one offending field:
// the message is "<path>: <reason>" and the status carries an
// io.altessa.badrequest.v1.BadRequest detail with one violation (field path
// and message, no machine code), the detail type the go-atlas validation
// interceptors emit and the go-atlas client parses. It is the status the
// field-error converters ([WithFieldErrorType], [WithCanonicalRules]) build.
func BadRequest(path, reason string) *status.Status {
	st := status.New(codes.InvalidArgument, path+": "+reason)
	detailed, err := st.WithDetails(&badrequestv1.BadRequest{
		FieldViolations: []*badrequestv1.FieldViolation{{FieldPath: new(path), Message: new(reason)}},
	})
	if err != nil {
		return st
	}
	return detailed
}

// FieldViolation returns the [BadRequest] status for one offending field as
// an error.
func FieldViolation(path, reason string) error {
	return BadRequest(path, reason).Err()
}

// PathMapper maps a domain field path (e.g. "base_url") to the API field path
// a violation names in the request (e.g. "provider.baseUrl").
type PathMapper func(field string) string

// CamelPath converts every dot-separated segment of path to lowerCamelCase
// with core/text/strings.ToCamelCase: underscores and whitespace start a new
// upper-case word and the first letter of a segment is lowered
// ("attrs.law_code" becomes "attrs.lawCode", "times[0]" stays "times[0]").
// A lowerCamelCase path comes back unchanged, so CamelPath is idempotent.
func CamelPath(path string) string {
	if path == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(path))
	for i, seg := range strings.Split(path, ".") {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(corestrings.ToCamelCase(seg))
	}
	return b.String()
}

// Prefixed returns a [PathMapper] that camel-cases a domain path with
// [CamelPath] and puts it under prefix, the request field holding the
// resource ("provider" maps "base_url" to "provider.baseUrl"). The prefix is
// used as written; an empty prefix only camel-cases.
func Prefixed(prefix string) PathMapper {
	if prefix == "" {
		return CamelPath
	}
	return func(field string) string {
		return prefix + "." + CamelPath(field)
	}
}
