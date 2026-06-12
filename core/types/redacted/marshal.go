// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted

import (
	"encoding/json"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"
)

// MarshalJSON implements json.Marshaler, emitting the placeholder as a
// JSON string. The underlying secret is never written.
func (s RedactedString) MarshalJSON() ([]byte, error) {
	return json.Marshal(placeholder)
}

// UnmarshalJSON implements json.Unmarshaler. It accepts any JSON
// string and stores it verbatim into the underlying value, so config
// loaders and Mongo decoders that route through json.Unmarshal restore
// the plain text. A non-string JSON value (object, array, number,
// null, bool) is rejected.
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

// UnmarshalYAML implements yaml.Unmarshaler. It expects a scalar
// string node and stores its value verbatim into the underlying
// RedactedString.
func (s *RedactedString) UnmarshalYAML(node *yaml.Node) error {
	var v string
	if err := node.Decode(&v); err != nil {
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
	typ, data, err := bson.MarshalValue(placeholder)
	if err != nil {
		return 0, nil, fmt.Errorf("redacted: marshal bson: %w", err)
	}
	return byte(typ), data, nil
}

// UnmarshalBSONValue implements bson.ValueUnmarshaler. It expects a
// BSON string and stores it verbatim into the underlying value. BSON
// null clears the value to the empty string. Other BSON types are
// rejected.
func (s *RedactedString) UnmarshalBSONValue(typ byte, data []byte) error {
	if bson.Type(typ) == bson.TypeNull {
		*s = ""
		return nil
	}
	var v string
	if err := bson.UnmarshalValue(bson.Type(typ), data, &v); err != nil {
		return fmt.Errorf("redacted: unmarshal bson: %w", err)
	}
	*s = RedactedString(v)
	return nil
}
