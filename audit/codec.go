// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"encoding/json"

	"github.com/altessa-s/go-atlas/audit/internal/jsonx"
)

// JSONCodec serializes Events to/from JSON for WAL durability.
// Recovered JSON records survive struct field additions without re-encoding,
// and free-form numeric values keep their exact representation via [json.Number].
type JSONCodec struct{}

// Encode serializes e to JSON bytes.
func (JSONCodec) Encode(e *Event) ([]byte, error) {
	return json.Marshal(e)
}

// Decode deserializes JSON bytes into an Event, preserving exact number literals
// in free-form maps.
func (JSONCodec) Decode(b []byte) (*Event, error) {
	var e Event
	if err := jsonx.Decode(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
