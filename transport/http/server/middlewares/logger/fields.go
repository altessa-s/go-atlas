// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	loggerfields "github.com/altessa-s/go-atlas/transport/internal/observability"
)

// HTTP-specific field keys used by the logger [Middleware].
// Interned via [strings.InternString] for memory efficiency, since these
// strings appear in every logged request entry.
var (
	FieldKeyHTTPMethod loggerfields.FieldKey = corestrings.InternString("http.method")
	FieldKeyHTTPPath   loggerfields.FieldKey = corestrings.InternString("http.path")
	FieldKeyHTTPProto  loggerfields.FieldKey = corestrings.InternString("http.proto")
	FieldKeyHTTPStatus loggerfields.FieldKey = corestrings.InternString("http.status")
)
