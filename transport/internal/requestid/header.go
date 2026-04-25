// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

// HeaderGetter abstracts single-valued header access so that [Generator]
// can work with both HTTP headers and gRPC metadata without importing
// either transport directly.
type HeaderGetter interface {
	// GetHeader returns the first value for the given header name,
	// or an empty string when the header is absent.
	GetHeader(name string) string
}
