// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"

	loggerfields "github.com/altessa-s/go-atlas/internal/transport/observability"
)

// HTTP-specific field keys used by the logger [Middleware].
// Interned via [intern.String] for memory efficiency, since these
// strings appear in every logged request entry.
var (
	FieldKeyHTTPMethod loggerfields.FieldKey = intern.String("http.method")
	FieldKeyHTTPPath   loggerfields.FieldKey = intern.String("http.path")
	FieldKeyHTTPProto  loggerfields.FieldKey = intern.String("http.proto")
	FieldKeyHTTPStatus loggerfields.FieldKey = intern.String("http.status")
	FieldKeyHTTPQuery  loggerfields.FieldKey = intern.String("http.query")
)
