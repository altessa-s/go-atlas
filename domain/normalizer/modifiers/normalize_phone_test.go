// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePhone(t *testing.T) {
	// Clear phone cache between runs
	phoneCache.Purge()

	tests := []struct {
		name      string
		in        any
		params    map[string]string
		wantErr   error
		wantValue string
	}{
		{"valid RU", "+79161234567", nil, nil, "+79161234567"},
		{"valid US with region", "2025551234", map[string]string{"region": "US"}, nil, "+12025551234"},
		{"invalid", "abc", nil, ErrPhoneNoValidDigits, ""},
		{"empty string", "", nil, nil, ""},
		{"non-string type", 42, nil, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePhone(reflect.ValueOf(tt.in), tt.params)
			if tt.wantErr != nil {
				require.NotNil(t, result.Error)
				require.True(t, errors.Is(result.Error, tt.wantErr))
				return
			}
			require.Nil(t, result.Error)
			if tt.wantValue != "" {
				require.Equal(t, tt.wantValue, result.Value.String())
			}
		})
	}
}

func TestNormalizePhone_Pointer(t *testing.T) {
	phoneCache.Purge()

	phone := "+79161234567"
	result := NormalizePhone(reflect.ValueOf(&phone), nil)
	require.Nil(t, result.Error)
}
