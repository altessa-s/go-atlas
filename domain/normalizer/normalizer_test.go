// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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

		require.NoError(t, normalizer.Normalize(user))

		require.Equal(t, "antonio", user.Name)
		require.Equal(t, "test@example.com", user.Email)
		require.NotNil(t, user.Nickname)
		require.Equal(t, "NICK", *user.Nickname)
		require.Equal(t, "  Keep  ", user.Ignored)
		require.Equal(t, "  Keep  ", user.NoTag)
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

		require.NoError(t, normalizer.Normalize(g))

		require.Equal(t, "Team", g.Label)
		require.Equal(t, "leader", g.Leader.Name)
		require.Len(t, g.Members, 2)
		require.Equal(t, "member1", g.Members[0].Name)
	})

	t.Run("Nil Pointers", func(t *testing.T) {
		var u *User
		require.Error(t, normalizer.Normalize(u))

		validU := &User{Name: "valid"}
		// Member with nil pointer field (Nickname) should be fine
		validU.Nickname = nil
		require.NoError(t, normalizer.Normalize(validU))
	})

	t.Run("Invalid Input", func(t *testing.T) {
		require.Error(t, normalizer.Normalize(User{}))
		require.Error(t, normalizer.Normalize("string"))
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
	require.NoError(t, normalizer.Normalize(c))
	require.Equal(t, "CUSTOM: test", c.Name)
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
	require.NoError(t, normalizer.Normalize(ts))
	require.Equal(t, "A_val", ts.Val)
}
