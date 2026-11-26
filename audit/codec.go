// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"bytes"
	"encoding/json"
	"io"
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
	if err := UnmarshalJSON(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// UnmarshalJSON decodes JSON data into dst with [json.Decoder.UseNumber] so
// numbers inside free-form maps stay [json.Number] instead of rounding through
// float64, while still rejecting malformed or trailing JSON input with the
// standard [encoding/json] syntax errors.
func UnmarshalJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		var raw json.RawMessage
		if syntaxErr := json.Unmarshal(data, &raw); syntaxErr != nil {
			return syntaxErr
		}
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		var raw json.RawMessage
		if syntaxErr := json.Unmarshal(data, &raw); syntaxErr != nil {
			return syntaxErr
		}
		return err
	}
	return nil
}
