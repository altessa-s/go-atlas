// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serializer_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestJSON(t *testing.T) {
	s := &serializer.JSON{}

	val := testhelpers.Person{Name: "Antonio", Age: 30}

	bytes, err := s.Serialize(val)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	var loaded testhelpers.Person
	if err := s.Deserialize(bytes, &loaded); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if !reflect.DeepEqual(val, loaded) {
		t.Errorf("Roundtrip failed: got %v, want %v", loaded, val)
	}
}
