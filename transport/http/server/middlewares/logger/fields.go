// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// FieldKey names a structured log attribute.
type FieldKey = string

// HTTP-specific field keys used by the logger [Middleware].
// Interned via [intern.String] for memory efficiency, since these
// strings appear in every logged request entry.
var (
	FieldKeyHTTPMethod FieldKey = intern.String("http.method")
	FieldKeyHTTPPath   FieldKey = intern.String("http.path")
	FieldKeyHTTPProto  FieldKey = intern.String("http.proto")
	FieldKeyHTTPStatus FieldKey = intern.String("http.status")
	FieldKeyHTTPQuery  FieldKey = intern.String("http.query")
)
