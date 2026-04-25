// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import "io"

// StreamingEncoder can encode data directly to an io.Writer without buffering.
// This is useful for large responses where memory efficiency is important.
//
// Implementations should write encoded data directly to the writer,
// avoiding intermediate byte slice allocation.
type StreamingEncoder interface {
	// EncodeStream writes encoded data directly to w.
	// The implementation should not buffer the entire response in memory.
	EncodeStream(w io.Writer, data any) error

	// ContentType returns the MIME type for this encoder.
	ContentType() string
}

// StreamingCodec combines streaming encoder with regular decoder.
// Use this interface when you need both streaming encoding and regular decoding.
type StreamingCodec interface {
	StreamingEncoder
	Decoder
}
