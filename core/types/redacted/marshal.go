// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

const (
	bsonStringType byte = 0x02
	bsonNullType   byte = 0x0a
)

// MarshalJSON implements json.Marshaler, emitting the placeholder as a
// JSON string. The underlying secret is never written.
func (s RedactedString) MarshalJSON() ([]byte, error) {
	return json.Marshal(placeholder)
}

// UnmarshalJSON implements json.Unmarshaler. It accepts any JSON
// string and stores it verbatim into the underlying value, so config
// loaders and Mongo decoders that route through json.Unmarshal restore
// the plain text. JSON null clears the value; any other non-string
// JSON value (object, array, number, bool) is rejected.
func (s *RedactedString) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("redacted: unmarshal json: %w", err)
	}
	*s = RedactedString(v)
	return nil
}

// MarshalYAML implements yaml.Marshaler, emitting the placeholder
// scalar. Used by gopkg.in/yaml.v3 — when a struct is YAML-encoded for
// diagnostic output, RedactedString fields render as <redacted>.
func (s RedactedString) MarshalYAML() (any, error) {
	return placeholder, nil
}

// UnmarshalYAML uses the callback unmarshaler supported by yaml.v3.
// Keeping the signature structural avoids a YAML dependency in this type.
func (s *RedactedString) UnmarshalYAML(unmarshal func(any) error) error {
	var v string
	if err := unmarshal(&v); err != nil {
		return fmt.Errorf("redacted: unmarshal yaml: %w", err)
	}
	*s = RedactedString(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler, returning the
// placeholder. This is the path used by url.Values, http.Header
// formatting, log/slog text handlers via TextMarshaler fallback, and
// other libraries that call MarshalText before falling back to
// fmt.Stringer.
func (s RedactedString) MarshalText() ([]byte, error) {
	return []byte(placeholder), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, storing the
// incoming bytes verbatim as the underlying value.
func (s *RedactedString) UnmarshalText(data []byte) error {
	*s = RedactedString(data)
	return nil
}

// MarshalBSONValue implements bson.ValueMarshaler, emitting the
// placeholder as a BSON string value. Combined with IsZero, a field
// tagged `bson:"field,omitempty"` is omitted entirely when the
// RedactedString is empty.
func (s RedactedString) MarshalBSONValue() (byte, []byte, error) {
	// BSON string: little-endian length (including NUL), UTF-8 bytes, NUL.
	// This fixed redaction encoding must work even without a custom registry.
	const headerSize = 4
	data := make([]byte, headerSize+len(placeholder)+1)
	binary.LittleEndian.PutUint32(data, uint32(len(placeholder)+1))
	copy(data[headerSize:], placeholder)
	return bsonStringType, data, nil
}

// UnmarshalBSONValue implements bson.ValueUnmarshaler. It expects a
// BSON string and stores it verbatim into the underlying value. BSON
// null clears the value to the empty string. Other BSON types are
// rejected.
func (s *RedactedString) UnmarshalBSONValue(typ byte, data []byte) error {
	if typ == bsonNullType {
		*s = ""
		return nil
	}
	const headerSize = 4
	if typ != bsonStringType || len(data) < headerSize+1 ||
		uint64(binary.LittleEndian.Uint32(data)) != uint64(len(data)-headerSize) || data[len(data)-1] != 0 {
		return fmt.Errorf("redacted: invalid BSON string")
	}
	*s = RedactedString(data[headerSize : len(data)-1])
	return nil
}
