// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

//go:generate go tool optgen generate --type=options

import "uuid"

const (
	// DefaultHTTPHeaderName is the default HTTP header name for request ID:
	// X-Request-ID, the de-facto standard, as the HTTP and gRPC server
	// configurations default to.
	DefaultHTTPHeaderName = "X-Request-ID"
)

// UUIDGenerator is the signature for functions that produce UUID strings.
// The default implementation uses [uuid.New].
type UUIDGenerator func() string

// defaultUUIDGenerator is the default UUID generator using the standard uuid package.
func defaultUUIDGenerator() string {
	return uuid.New().String()
}

// options holds [Generator] configuration populated by functional
// [Option] values.
type options struct {
	headerName        string        `optgen:"default=DefaultHTTPHeaderName"`
	generateIfMissing bool          `optgen:"default=true" optval:"param"`
	uuidGenerator     UUIDGenerator `optgen:"default=defaultUUIDGenerator"`
}
