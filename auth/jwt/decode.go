// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/altessa-s/go-atlas/auth/internal/claimnum"
	"github.com/altessa-s/go-atlas/auth/internal/numericdate"
)

// decodeClaims decodes the single JSON value supplied by UnmarshalJSON.
// Registered dates retain their signed number spelling; other numbers are
// normalized by [claimnum.Value].
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
			v, err := claimnum.Value(value)
			if err != nil {
				return nil, err
			}
			claims[key] = v
		}
	}
	return claims, nil
}
