// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serializer

import (
	"encoding/json"
)

// JSON is a [Serializer] implementation backed by [encoding/json].
// It delegates to [json.Marshal] and [json.Unmarshal] for encoding and
// decoding respectively, so all standard library JSON rules apply
// (struct tags, custom marshalers, etc.).
//
// JSON is stateless and safe for concurrent use from multiple goroutines.
// A zero value is ready to use.
//
// Example:
//
//	s := &serializer.JSON{}
//	bytes, err := s.Serialize(&user)
//	var loaded User
//	err = s.Deserialize(bytes, &loaded)
type JSON struct{}

// Serialize encodes data as JSON using [json.Marshal] and returns the
// resulting bytes. It returns an error if data contains values that cannot
// be represented in JSON (e.g., channels, complex numbers, or cycles).
func (s *JSON) Serialize(data any) ([]byte, error) {
	return json.Marshal(data)
}

// Deserialize decodes the JSON-encoded bytes in d into out using
// [json.Unmarshal]. The out parameter must be a non-nil pointer;
// passing a non-pointer or nil pointer causes a runtime error from
// the underlying [json.Unmarshal] call. It returns an error if d is
// not valid JSON or cannot be mapped to the target type.
func (s *JSON) Deserialize(d []byte, out any) error {
	return json.Unmarshal(d, out)
}

var _ Serializer = (*JSON)(nil)
