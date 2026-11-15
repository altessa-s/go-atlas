// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

const (
	// DefaultGRPCMetadataKey is the default gRPC metadata key for request ID:
	// [DefaultHTTPHeaderName] in the lower case gRPC metadata keys use.
	DefaultGRPCMetadataKey = "x-request-id"
)

// InternedHTTPHeaderName is the interned form of [DefaultHTTPHeaderName],
// used on hot paths to avoid per-request string allocations.
var InternedHTTPHeaderName = intern.String(DefaultHTTPHeaderName)

// InternedGRPCMetadataKey is the interned form of [DefaultGRPCMetadataKey],
// used on hot paths to avoid per-request string allocations.
var InternedGRPCMetadataKey = intern.String(DefaultGRPCMetadataKey)
