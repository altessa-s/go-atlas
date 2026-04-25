// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import (
	jsoncodec "github.com/altessa-s/go-atlas/transport/http/server/codec/providers/json"
	xmlcodec "github.com/altessa-s/go-atlas/transport/http/server/codec/providers/xml"
)

func init() {
	// Register default codecs in the global registry
	registry := DefaultRegistry()

	// Register JSON codec
	jsonCodec := jsoncodec.New()
	registry.MustRegisterCodec(jsonCodec)

	// Register alternate JSON MIME type
	_ = registry.RegisterEncoder(jsoncodec.AlternateMimeType, jsonCodec) //nolint:errcheck // Already registered
	_ = registry.RegisterDecoder(jsoncodec.AlternateMimeType, jsonCodec) //nolint:errcheck // Already registered

	// Register XML codec
	xmlCodec := xmlcodec.New()
	registry.MustRegisterCodec(xmlCodec)

	// Register alternate XML MIME type
	_ = registry.RegisterEncoder(xmlcodec.AlternateMimeType, xmlCodec) //nolint:errcheck // Already registered
	_ = registry.RegisterDecoder(xmlcodec.AlternateMimeType, xmlCodec) //nolint:errcheck // Already registered
}
