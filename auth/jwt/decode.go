// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/altessa-s/go-atlas/auth/internal/numericdate"
)

// decodeClaims decodes the single JSON value supplied by UnmarshalJSON.
// Only registered dates retain their signed number spelling.
func decodeClaims(data []byte) (map[string]any, error) {
	var claims map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&claims); err != nil {
		return nil, err
	}
	for key, value := range claims {
		switch key {
		case claimExpiration, claimIssuedAt, claimNotBefore:
			if _, ok := numericdate.Parse(value); !ok {
				return nil, errors.New("invalid registered date: " + key)
			}
		default:
			v, err := customValue(value)
			if err != nil {
				return nil, err
			}
			claims[key] = v
		}
	}
	return claims, nil
}

// customValue restores the encoding/json representation of private claims.
func customValue(value any) (any, error) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil, errors.New("custom numeric claim outside float64 range")
		}
		return f, nil
	case map[string]any:
		for k, item := range v {
			decoded, err := customValue(item)
			if err != nil {
				return nil, err
			}
			v[k] = decoded
		}
	case []any:
		for i, item := range v {
			decoded, err := customValue(item)
			if err != nil {
				return nil, err
			}
			v[i] = decoded
		}
	}
	return value, nil
}
