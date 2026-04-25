// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

// Encoder serializes arbitrary data into bytes for a specific MIME type.
type Encoder interface {
	// Encode serializes data and returns the encoded bytes.
	Encode(data any) ([]byte, error)

	// ContentType returns the MIME type produced by this encoder
	// (e.g., "application/json").
	ContentType() string
}

// Decoder deserializes bytes into a Go value for a specific MIME type.
type Decoder interface {
	// Decode deserializes data into out, which must be a pointer.
	Decode(data []byte, out any) error

	// ContentType returns the MIME type consumed by this decoder.
	ContentType() string
}

// Codec combines [Encoder] and [Decoder] for a single MIME type.
type Codec interface {
	Encoder
	Decoder
}
