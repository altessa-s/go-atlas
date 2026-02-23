// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"
)

func TestRegisterAndGetModifier(t *testing.T) {
	RegisterModifier("test_mod", func(_ reflect.Value, _ map[string]string) ModifierResult {
		return ModifierResult{}
	})
	mod, ok := GetModifier("test_mod")
	if !ok || mod == nil {
		t.Fatal("expected modifier to be registered")
	}
}

func TestGetModifier_NotFound(t *testing.T) {
	_, ok := GetModifier("nonexistent_modifier_xyz")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestBuiltinModifiersRegistered(t *testing.T) {
	for _, name := range []string{"lowercase", "uppercase", "trim", "nil_on_empty", "phone"} {
		t.Run(name, func(t *testing.T) {
			_, ok := GetModifier(name)
			if !ok {
				t.Fatalf("builtin modifier %q not registered", name)
			}
		})
	}
}
