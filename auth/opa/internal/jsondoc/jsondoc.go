// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsondoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Decode reads exactly one JSON document, preserving numeric precision.
func Decode(content []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("unexpected token after JSON document")
	}
	return value, nil
}
