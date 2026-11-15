// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHasExportedField(t *testing.T) {
	t.Parallel()

	type exported struct{ A string }
	type unexported struct{ a string } //nolint:unused // exercised via reflection
	type empty struct{}

	tests := []struct {
		name   string
		target any
		want   bool
	}{
		{"struct with exported field", exported{}, true},
		{"struct with only unexported fields", unexported{}, false},
		{"empty struct", empty{}, false},
		{"time.Time", time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, hasExportedField(reflect.TypeOf(tt.target)))
		})
	}
}
