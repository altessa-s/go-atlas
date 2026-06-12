// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// pathCollector is a minimal [behavior.Translator] that flattens the resolved
// tree into leaf paths, marking stripped leaves with a trailing "!". It exercises
// the engine's walk, Strip flags, and nested descent.
type pathCollector struct{}

func (pathCollector) Translate(_ context.Context, root behavior.Object) ([]string, error) {
	var out []string
	var walk func(o behavior.Object, prefix string)
	walk = func(o behavior.Object, prefix string) {
		for _, f := range o.Fields {
			p := prefix + f.Name
			switch {
			case f.Strip:
				out = append(out, p+"!")
			case f.Nested != nil:
				walk(*f.Nested, p+".")
			case f.Collection != nil:
				for i, it := range f.Collection.Items {
					walk(it, fmt.Sprintf("%s[%d].", p, i))
				}
			default:
				out = append(out, p)
			}
		}
	}
	walk(root, "")
	return out, nil
}

func TestEngineTranslateDrivesTranslator(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type outer struct {
		ID    string `behavior:"identifier"`
		Child inner
	}

	eng := behavior.New[[]string](pathCollector{}, behavior.WithKinds(behavior.Identifier))
	got, err := eng.Translate(t.Context(), &outer{Child: inner{Secret: "s", Keep: "k"}})
	require.NoError(t, err)

	// ID is stripped (Identifier); Child is descended; input_only is not in the
	// strip set so its leaf is plain.
	require.ElementsMatch(t, []string{"ID!", "Child.Secret", "Child.Keep"}, got)
}

func TestEngineTranslateSchemaWalkResolvesAbsentNested(t *testing.T) {
	t.Parallel()

	type leaf struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type root struct {
		Ptr   *leaf            // nil pointer-to-struct
		List  []leaf           // empty slice of structs
		ByKey map[string]*leaf // nil map with struct values
	}

	// Without schema-walk the absent nested values are not descended: each field
	// is reported as a single plain leaf, exposing no nested paths.
	plain := behavior.New[[]string](pathCollector{}, behavior.WithKinds(behavior.InputOnly))
	got, err := plain.Translate(t.Context(), &root{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Ptr", "List", "ByKey"}, got)

	// With schema-walk each nested struct type contributes one representative
	// element, so its leaf paths appear even though every value is absent.
	walk := behavior.New[[]string](pathCollector{},
		behavior.WithKinds(behavior.InputOnly), behavior.WithSchemaWalk())
	got, err = walk.Translate(t.Context(), &root{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"Ptr.Secret!", "Ptr.Keep",
		"List[0].Secret!", "List[0].Keep",
		"ByKey[0].Secret!", "ByKey[0].Keep",
	}, got)
}

func TestEngineTranslateRejectsNonStruct(t *testing.T) {
	t.Parallel()

	eng := behavior.New[[]string](pathCollector{})

	_, err := eng.Translate(t.Context(), nil)
	require.Error(t, err)

	n := 5
	_, err = eng.Translate(t.Context(), &n)
	require.Error(t, err)

	var p *struct{ X int }
	_, err = eng.Translate(t.Context(), p)
	require.Error(t, err, "typed nil pointer is rejected")
}

func TestCleanDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type outer struct {
		Ptr   *inner
		List  []inner
		ByKey map[string]inner
	}

	orig := outer{
		Ptr:   &inner{Secret: "s", Keep: "k"},
		List:  []inner{{Secret: "s", Keep: "k"}},
		ByKey: map[string]inner{"a": {Secret: "s", Keep: "k"}},
	}

	cleaned, err := behavior.Clean(orig, behavior.WithKinds(behavior.InputOnly))
	require.NoError(t, err)

	// Copy is stripped.
	require.Empty(t, cleaned.Ptr.Secret)
	require.Empty(t, cleaned.List[0].Secret)
	require.Empty(t, cleaned.ByKey["a"].Secret)
	require.Equal(t, "k", cleaned.Ptr.Keep)

	// Original is fully intact, including data reached through pointer, slice,
	// and map — the deep copy isolated it.
	require.Equal(t, "s", orig.Ptr.Secret, "original pointee untouched")
	require.Equal(t, "s", orig.List[0].Secret, "original slice backing untouched")
	require.Equal(t, "s", orig.ByKey["a"].Secret, "original map value untouched")
}

func TestCleanInterfaceFieldSharedNotMutated(t *testing.T) {
	t.Parallel()

	type holder struct {
		Token any `behavior:"input_only"`
		Keep  any
	}

	shared := &struct{ V string }{V: "v"}
	orig := holder{Token: shared, Keep: shared}

	cleaned, err := behavior.Clean(orig, behavior.WithKinds(behavior.InputOnly))
	require.NoError(t, err)

	// A tagged interface field is cleared whole in the copy; an untagged one is a
	// leaf and shares its pointee with the original instead of being deep-cloned.
	require.Nil(t, cleaned.Token)
	require.Same(t, shared, cleaned.Keep)
	require.Same(t, shared, orig.Token, "original untouched")
	require.Equal(t, "v", shared.V)
}

func TestCleanNoKindsReturnsCopy(t *testing.T) {
	t.Parallel()

	type m struct {
		ID string `behavior:"identifier"`
	}

	in := m{ID: "x"}
	out, err := behavior.Clean(in)
	require.NoError(t, err)
	require.Equal(t, in, out, "without WithKinds Clean is an identity copy")
}
