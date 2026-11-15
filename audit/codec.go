// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "encoding/json"

// JSONCodec serializes Events to/from JSON for WAL durability.
// Recovered JSON records survive struct field additions without re-encoding.
type JSONCodec struct{}

func (JSONCodec) Encode(e *Event) ([]byte, error) {
	return json.Marshal(e)
}

func (JSONCodec) Decode(b []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
