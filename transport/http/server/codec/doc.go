// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package codec provides HTTP body encoding/decoding with content negotiation.
//
// The package defines [Encoder], [Decoder], and [Codec] interfaces for
// serialization, along with a thread-safe [Registry] that maps MIME types
// to codec implementations. The init function registers JSON and XML codecs
// (with alternate MIME types) in the global [DefaultRegistry].
//
// Content negotiation is performed by [Registry.Negotiate], which parses the
// HTTP Accept header and selects the best matching encoder.
//
// For large responses, [StreamingEncoder] and [StreamingCodec] allow writing
// directly to an [io.Writer] without intermediate buffering.
//
// # Example
//
//	enc, mimeType, _ := codec.DefaultRegistry().Negotiate(acceptHeader)
//	data, _ := enc.Encode(response)
package codec
