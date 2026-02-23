// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"

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
	if len(fields) != 1 {
		t.Errorf("changed fields = %v, want 1 field", fields)
	}
}

func TestGetChangedFields_WithIgnoreFields(t *testing.T) {
	type S struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	before := S{Name: "Alice", Age: 30}
	after := S{Name: "Bob", Age: 40}

	fields := fieldtracker.GetChangedFields(before, after, fieldtracker.WithIgnoreFields("name"))
	for _, f := range fields {
		if f == "name" {
			t.Error("ignored field 'name' should not appear in changed fields")
		}
	}
}

func TestTracker_Reuse(t *testing.T) {
	type S struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	tracker := fieldtracker.NewTracker()
	fields1 := tracker.GetChangedFields(S{X: 1, Y: 2}, S{X: 1, Y: 3})
	fields2 := tracker.GetChangedFields(S{X: 1, Y: 2}, S{X: 2, Y: 2})

	if len(fields1) != 1 || len(fields2) != 1 {
		t.Errorf("fields1=%v, fields2=%v, want 1 each", fields1, fields2)
	}
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
	if len(fields) == 0 {
		t.Error("should detect nested field change")
	}
}
