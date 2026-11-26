// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Decode unmarshals a single JSON value from data into dst with
// [json.Decoder.UseNumber] so numbers inside free-form maps stay [json.Number]
// instead of rounding through float64, while returning standard [encoding/json]
// syntax errors for truncated or trailing input.
func Decode(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		var raw json.RawMessage
		if syntaxErr := json.Unmarshal(data, &raw); syntaxErr != nil {
			return syntaxErr
		}
		return err
	}
	if dec.More() {
		return trailingInputError(data)
	}
	if _, err := dec.Token(); err != io.EOF {
		return trailingInputError(data)
	}
	return nil
}

func trailingInputError(data []byte) error {
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return errors.New("jsonx: unexpected trailing JSON input")
}
