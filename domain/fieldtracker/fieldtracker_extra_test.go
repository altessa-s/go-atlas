// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/fieldtracker"
)

func TestGetChangedFields_WithTagNamePtr(t *testing.T) {
	tag := "yaml"
	type S struct {
		Name string `yaml:"name"`
		Age  int    `yaml:"age"`
	}
	before := S{Name: "Alice", Age: 30}
	after := S{Name: "Bob", Age: 30}

	fields := fieldtracker.GetChangedFields(before, after, fieldtracker.WithTagName(&tag))
	require.Len(t, fields, 1)
}

func TestGetChangedFields_WithIgnoreFields(t *testing.T) {
	type S struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	before := S{Name: "Alice", Age: 30}
	after := S{Name: "Bob", Age: 40}

	fields := fieldtracker.GetChangedFields(before, after, fieldtracker.WithIgnoreFields("name"))
	require.NotContains(t, fields, "name")
}

func TestTracker_Reuse(t *testing.T) {
	type S struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	tracker := fieldtracker.NewTracker()
	fields1 := tracker.GetChangedFields(S{X: 1, Y: 2}, S{X: 1, Y: 3})
	fields2 := tracker.GetChangedFields(S{X: 1, Y: 2}, S{X: 2, Y: 2})

	require.Len(t, fields1, 1)
	require.Len(t, fields2, 1)
}

func TestGetChangedFields_Nested(t *testing.T) {
	type Inner struct {
		Val string `json:"val"`
	}
	type Outer struct {
		Inner Inner `json:"inner"`
	}
	before := Outer{Inner: Inner{Val: "a"}}
	after := Outer{Inner: Inner{Val: "b"}}

	fields := fieldtracker.GetChangedFields(before, after)
	require.NotEmpty(t, fields, "should detect nested field change")
}
