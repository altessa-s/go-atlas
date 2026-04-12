// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serializer_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestJSON(t *testing.T) {
	s := &serializer.JSON{}

	val := testhelpers.Person{Name: "Antonio", Age: 30}

	bytes, err := s.Serialize(val)
	require.NoError(t, err, "Serialize failed")

	var loaded testhelpers.Person
	require.NoError(t, s.Deserialize(bytes, &loaded), "Deserialize failed")

	require.True(t, reflect.DeepEqual(val, loaded), "Roundtrip failed: got %v, want %v", loaded, val)
}
