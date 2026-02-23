// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/domain/normalizer"
	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

type User struct {
	Name     string  `normalize:"trim,lowercase"`
	Email    string  `normalize:"trim,lowercase"`
	Nickname *string `normalize:"trim,uppercase"`
	Ignored  string  `normalize:"-"`
	NoTag    string
}

type Group struct {
	Label   string `normalize:"trim"`
	Leader  User
	Members []User
}

func TestNormalize(t *testing.T) {
	t.Run("Basic String Modifiers", func(t *testing.T) {
		nick := "  nick  "
		user := &User{
			Name:     "  Antonio  ",
			Email:    "  TEST@Example.com ",
			Nickname: &nick,
			Ignored:  "  Keep  ",
			NoTag:    "  Keep  ",
		}

		if err := normalizer.Normalize(user); err != nil {
			t.Fatalf("Normalize failed: %v", err)
		}

		if user.Name != "antonio" {
			t.Errorf("Name not normalized: got %q", user.Name)
		}
		if user.Email != "test@example.com" {
			t.Errorf("Email not normalized: got %q", user.Email)
		}
		if user.Nickname == nil || *user.Nickname != "NICK" {
			t.Errorf("Nickname not normalized: got %q", *user.Nickname)
		}
		if user.Ignored != "  Keep  " {
			t.Errorf("Ignored field modified: %q", user.Ignored)
		}
		if user.NoTag != "  Keep  " {
			t.Errorf("NoTag field modified: %q", user.NoTag)
		}
	})

	t.Run("Nested Structs and Slices", func(t *testing.T) {
		g := &Group{
			Label: "  Team  ",
			Leader: User{
				Name: "  Leader  ",
			},
			Members: []User{
				{Name: "  Member1  "},
				{Name: "  Member2  "},
			},
		}

		if err := normalizer.Normalize(g); err != nil {
			t.Fatalf("Normalize failed: %v", err)
		}

		if g.Label != "Team" {
			t.Errorf("Label not normalized: %q", g.Label)
		}
		if g.Leader.Name != "leader" {
			t.Errorf("Leader name not normalized: %q", g.Leader.Name)
		}
		if len(g.Members) != 2 {
			t.Fatalf("Members count changed: %d", len(g.Members))
		}
		if g.Members[0].Name != "member1" {
			t.Errorf("Member1 not normalized: %q", g.Members[0].Name)
		}
	})

	t.Run("Nil Pointers", func(t *testing.T) {
		var u *User
		if err := normalizer.Normalize(u); err == nil {
			t.Error("Expected error for nil pointer")
		}

		validU := &User{Name: "valid"}
		// Member with nil pointer field (Nickname) should be fine
		validU.Nickname = nil
		if err := normalizer.Normalize(validU); err != nil {
			t.Errorf("Normalize failed for struct with nil field: %v", err)
		}
	})

	t.Run("Invalid Input", func(t *testing.T) {
		if err := normalizer.Normalize(User{}); err == nil {
			t.Error("Expected error for non-pointer struct")
		}
		if err := normalizer.Normalize("string"); err == nil {
			t.Error("Expected error for non-struct")
		}
	})
}

// Custom Normalizer Test
type CustomUser struct {
	Name string
}

func (c *CustomUser) Normalize() error {
	c.Name = "CUSTOM: " + strings.TrimSpace(c.Name)
	return nil
}

func TestCustomNormalizer(t *testing.T) {
	c := &CustomUser{Name: "  test  "}
	if err := normalizer.Normalize(c); err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}
	if c.Name != "CUSTOM: test" {
		t.Errorf("Custom normalizer not called, got: %q", c.Name)
	}
}

// Global Modifier Registration Test
func TestCustomModifierRegistration(t *testing.T) {
	modifiers.RegisterModifier("prefix_a", func(v reflect.Value, _ map[string]string) modifiers.ModifierResult {
		return modifiers.ApplyStringModifier(v, func(s string) (string, bool) {
			return "A_" + s, true
		})
	})

	type TestStruct struct {
		Val string `normalize:"prefix_a"`
	}

	ts := &TestStruct{Val: "val"}
	if err := normalizer.Normalize(ts); err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if ts.Val != "A_val" {
		t.Errorf("Custom modifier failed: got %q", ts.Val)
	}
}
