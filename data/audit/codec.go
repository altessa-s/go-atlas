// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "encoding/json"

// jsonCodec serializes Events to/from JSON for WAL durability.
// JSON is chosen over more compact formats for forward/backward compatibility:
// recovered records survive struct field additions without re-encoding.
type jsonCodec struct{}

func (jsonCodec) Encode(e *Event) ([]byte, error) {
	return json.Marshal(e)
}

func (jsonCodec) Decode(b []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
