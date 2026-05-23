// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import "github.com/google/uuid"

const (
	// DefaultHTTPHeaderName is the default HTTP header name for request ID.
	DefaultHTTPHeaderName = "Request-ID"
)

// UUIDGenerator is the signature for functions that produce UUID strings.
// The default implementation uses [github.com/google/uuid.New].
type UUIDGenerator func() string

// defaultUUIDGenerator is the default UUID generator using google/uuid.
func defaultUUIDGenerator() string {
	return uuid.New().String()
}

// options holds [Generator] configuration populated by functional
// [Option] values.
type options struct {
	headerName        string        `optgen:"default=DefaultHTTPHeaderName"`
	generateIfMissing bool          `optgen:"default=true"`
	uuidGenerator     UUIDGenerator `optgen:"default=defaultUUIDGenerator"`
}
