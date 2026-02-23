// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/fieldtracker"
)

func TestGetChangedFields_PackageLevel(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	before := &User{Name: "Alice", Email: "a@b.com"}
	after := &User{Name: "Bob", Email: "a@b.com"}

	changed := fieldtracker.GetChangedFields(before, after)
	if len(changed) != 1 || changed[0] != "name" {
		t.Errorf("GetChangedFields() = %v, want [name]", changed)
	}
}

func TestGetChangedFields_Equal(t *testing.T) {
	type S struct {
		V int `json:"v"`
	}
	s := &S{V: 1}
	changed := fieldtracker.GetChangedFields(s, s)
	if len(changed) != 0 {
		t.Errorf("equal structs should return no changes, got %v", changed)
	}
}

func TestGetChangedFields_Nil(t *testing.T) {
	type S struct {
		V int `json:"v"`
	}
	changed := fieldtracker.GetChangedFields((*S)(nil), (*S)(nil))
	if len(changed) != 0 {
		t.Errorf("nil structs should return no changes, got %v", changed)
	}
}

func TestGetChangedFields_WithTagName(t *testing.T) {
	type S struct {
		Name string `db:"user_name" json:"name"`
	}

	tr := fieldtracker.NewTracker(fieldtracker.WithTagName("db"))
	changed := tr.GetChangedFields(&S{Name: "a"}, &S{Name: "b"})
	if len(changed) != 1 || changed[0] != "user_name" {
		t.Errorf("WithTagName(db) changed = %v, want [user_name]", changed)
	}
}

func TestGetChangedFields_NestedStruct(t *testing.T) {
	type Address struct {
		City string `json:"city"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	before := &Person{Name: "A", Address: Address{City: "NYC"}}
	after := &Person{Name: "A", Address: Address{City: "LA"}}

	tr := fieldtracker.NewTracker()
	changed := tr.GetChangedFields(before, after)
	if len(changed) != 1 || changed[0] != "address.city" {
		t.Errorf("changed = %v, want [address.city]", changed)
	}
}

func TestGetChangedFields_SliceLengthDiff(t *testing.T) {
	type S struct {
		Items []string `json:"items"`
	}

	before := &S{Items: []string{"a", "b"}}
	after := &S{Items: []string{"a", "b", "c"}}

	tr := fieldtracker.NewTracker()
	changed := tr.GetChangedFields(before, after)
	if len(changed) == 0 {
		t.Error("expected changes for different slice lengths")
	}
}

func TestGetChangedFields_MapNewKey(t *testing.T) {
	type S struct {
		Data map[string]int `json:"data"`
	}

	before := &S{Data: map[string]int{"a": 1}}
	after := &S{Data: map[string]int{"a": 1, "b": 2}}

	tr := fieldtracker.NewTracker()
	changed := tr.GetChangedFields(before, after)
	if len(changed) == 0 {
		t.Error("expected changes for new map key")
	}
}

func TestGetChangedFields_PointerFields(t *testing.T) {
	type S struct {
		Val *int `json:"val"`
	}

	a, b := 1, 2
	before := &S{Val: &a}
	after := &S{Val: &b}

	tr := fieldtracker.NewTracker()
	changed := tr.GetChangedFields(before, after)
	if len(changed) != 1 || changed[0] != "val" {
		t.Errorf("changed = %v, want [val]", changed)
	}
}

func TestGetChangedFields_NilToValue(t *testing.T) {
	type S struct {
		Val *int `json:"val"`
	}

	v := 1
	before := &S{Val: nil}
	after := &S{Val: &v}

	tr := fieldtracker.NewTracker()
	changed := tr.GetChangedFields(before, after)
	if len(changed) != 1 || changed[0] != "val" {
		t.Errorf("changed = %v, want [val]", changed)
	}
}

func TestGetChangedFields_BoolField(t *testing.T) {
	type S struct {
		Active bool `json:"active"`
	}

	before := &S{Active: false}
	after := &S{Active: true}

	changed := fieldtracker.GetChangedFields(before, after)
	if len(changed) != 1 || changed[0] != "active" {
		t.Errorf("changed = %v, want [active]", changed)
	}
}
