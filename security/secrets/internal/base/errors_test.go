// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/internal/base"
)

func TestErrorWrapping(t *testing.T) {
	tests := []struct {
		name        string
		errorFunc   func(error) error
		sentinelErr error
	}{
		{"KeyEncodingError", base.KeyEncodingError, secrets.ErrKeyEncoding},
		{"KeyDecodingError", base.KeyDecodingError, secrets.ErrKeyDecoding},
		{"ValueEncodingError", base.ValueEncodingError, secrets.ErrEncoding},
		{"ValueDecodingError", base.ValueDecodingError, secrets.ErrDecoding},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := errors.New("original error")
			wrapped := tt.errorFunc(original)

			require.ErrorIs(t, wrapped, tt.sentinelErr)
			require.ErrorIs(t, wrapped, original)
		})
	}
}
