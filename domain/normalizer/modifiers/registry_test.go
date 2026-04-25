// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterAndGetModifier(t *testing.T) {
	RegisterModifier("test_mod", func(_ reflect.Value, _ map[string]string) ModifierResult {
		return ModifierResult{}
	})
	mod, ok := GetModifier("test_mod")
	require.True(t, ok)
	require.NotNil(t, mod)
}

func TestGetModifier_NotFound(t *testing.T) {
	_, ok := GetModifier("nonexistent_modifier_xyz")
	require.False(t, ok)
}

func TestBuiltinModifiersRegistered(t *testing.T) {
	for _, name := range []string{"lowercase", "uppercase", "trim", "nil_on_empty", "phone"} {
		t.Run(name, func(t *testing.T) {
			_, ok := GetModifier(name)
			require.True(t, ok, "builtin modifier %q not registered", name)
		})
	}
}
