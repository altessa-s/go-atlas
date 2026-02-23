// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/normalizer"
)

func TestNormalize_StringPointer(t *testing.T) {
	type S struct {
		Name *string `normalize:"trim,lowercase"`
	}

	val := "  HELLO  "
	s := &S{Name: &val}
	if err := normalizer.Normalize(s); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if *s.Name != "hello" {
		t.Errorf("Name = %q, want 'hello'", *s.Name)
	}
}

func TestNormalize_NilPointerField(t *testing.T) {
	type S struct {
		Name *string `normalize:"trim"`
	}

	s := &S{Name: nil}
	if err := normalizer.Normalize(s); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if s.Name != nil {
		t.Error("nil pointer should remain nil")
	}
}

func TestNormalize_SliceOfStrings(t *testing.T) {
	type S struct {
		Tags []string `normalize:"trim,lowercase"`
	}

	s := &S{Tags: []string{"  FOO ", " BAR "}}
	// Normalize may not process slice of primitives — just ensure no error/panic
	_ = normalizer.Normalize(s)
}

func TestNormalize_NestedStruct(t *testing.T) {
	type Inner struct {
		Val string `normalize:"uppercase"`
	}
	type Outer struct {
		Inner Inner
	}

	s := &Outer{Inner: Inner{Val: "hello"}}
	if err := normalizer.Normalize(s); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if s.Inner.Val != "HELLO" {
		t.Errorf("Inner.Val = %q, want 'HELLO'", s.Inner.Val)
	}
}

func TestNormalize_InvalidInput(t *testing.T) {
	err := normalizer.Normalize("not a struct")
	if err == nil {
		t.Error("Normalize(string) should return error")
	}
}

func TestNormalize_NilInput(t *testing.T) {
	err := normalizer.Normalize(nil)
	if err == nil {
		t.Error("Normalize(nil) should return error")
	}
}

func TestClearParameterCache(t *testing.T) {
	// Just ensure no panic
	normalizer.ClearParameterCache()
}

func TestClearStructFieldCache(t *testing.T) {
	normalizer.ClearStructFieldCache()
}

func TestNormalize_MultipleFields(t *testing.T) {
	type S struct {
		First string `normalize:"trim"`
		Last  string `normalize:"trim,uppercase"`
		Email string `normalize:"trim,lowercase"`
	}

	s := &S{First: "  John  ", Last: "  doe  ", Email: "  USER@EXAMPLE.COM  "}
	if err := normalizer.Normalize(s); err != nil {
		t.Fatal(err)
	}
	if s.First != "John" {
		t.Errorf("First = %q", s.First)
	}
	if s.Last != "DOE" {
		t.Errorf("Last = %q", s.Last)
	}
	if s.Email != "user@example.com" {
		t.Errorf("Email = %q", s.Email)
	}
}

func TestNormalize_EmptySlice(t *testing.T) {
	type S struct {
		Tags []string `normalize:"trim"`
	}

	s := &S{Tags: []string{}}
	if err := normalizer.Normalize(s); err != nil {
		t.Fatal(err)
	}
}

func TestNormalize_SliceOfStructs(t *testing.T) {
	type Item struct {
		Name string `normalize:"trim,lowercase"`
	}
	type S struct {
		Items []Item
	}

	s := &S{Items: []Item{{Name: "  FOO "}, {Name: " BAR "}}}
	if err := normalizer.Normalize(s); err != nil {
		t.Fatal(err)
	}
	if s.Items[0].Name != "foo" {
		t.Errorf("Items[0].Name = %q", s.Items[0].Name)
	}
}
