// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	"github.com/altessa-s/go-atlas/domain/converter"
	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

type privateState struct {
	Secret   string
	Name     string
	Optional *string
	state    int
}

func newPrivateState(secret, name string, state int) privateState {
	return privateState{Secret: secret, Name: name, state: state}
}
func (s privateState) State() int { return s.state }

func TestIgnorePathsPreservesPrivateState(t *testing.T) {
	t.Parallel()
	type root struct {
		Value   privateState
		Pointer *privateState
		Items   []privateState
		Map     map[string]privateState
		Array   [1]privateState
	}
	for _, tc := range []struct {
		name   string
		option converter.Option
		state  int
	}{
		{"overwrite", nil, 7},
		{"sparse", converter.WithSparseMerge(), 3},
		{"zero", converter.WithIgnoreZeroValues(), 3},
		{"nil", converter.WithIgnoreNilValues(), 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := newPrivateState("source secret", "source", 7)
			old := newPrivateState("old secret", "old", 3)
			optional := "keep"
			old.Optional = &optional
			src := root{source, &source, []privateState{source}, map[string]privateState{"k": source}, [1]privateState{source}}
			dst := root{Value: old, Pointer: &old}
			opts := []converter.Option{converter.WithIgnorePaths("value.secret", "pointer.secret", "items.secret", "map.secret", "array.secret")}
			if tc.option != nil {
				opts = append(opts, tc.option)
			}
			converter.Convert(src, &dst, opts...)
			for _, got := range []privateState{dst.Value, *dst.Pointer} {
				require.Equal(t, "old secret", got.Secret)
				require.Equal(t, "source", got.Name)
				require.Equal(t, tc.state, got.State())
				require.Equal(t, &optional, got.Optional)
			}
			if tc.option == nil {
				for _, got := range []privateState{dst.Items[0], dst.Map["k"], dst.Array[0]} {
					require.Equal(t, 7, got.State())
					require.Empty(t, got.Secret)
				}
			}
			require.Equal(t, "source secret", source.Secret)
			if tc.option == nil {
				require.Equal(t, "old", old.Name)
			}
		})
	}
}

func TestIgnorePathsDynamicValues(t *testing.T) {
	t.Parallel()
	type root struct{ Data any }
	type object struct {
		Secret string
		Name   string
	}
	var typedNil *object
	for _, tc := range []struct {
		name           string
		src, dst, want any
	}{
		{"struct", object{"secret", "new"}, object{"keep", "old"}, object{"keep", "new"}},
		{"pointer", &object{"secret", "new"}, &object{"keep", "old"}, &object{"keep", "new"}},
		{"map", map[string]any{"secret": "secret", "name": "new"}, map[string]any{"secret": "keep"}, map[string]any{"secret": "keep", "name": "new"}},
		{"map absent", map[string]any{"secret": "secret", "name": "new"}, nil, map[string]any{"name": "new"}},
		{"slice", []object{{"secret", "new"}}, nil, []object{{"", "new"}}},
		{"array", [1]object{{"secret", "new"}}, nil, [1]object{{"", "new"}}},
		{"scalar", 42, 1, 42},
		{"nil", nil, object{}, nil},
		{"typed nil", typedNil, nil, typedNil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src, dst := root{tc.src}, root{tc.dst}
			converter.Convert(src, &dst, converter.WithIgnorePaths("data.secret"))
			require.Equal(t, tc.want, dst.Data)
			if tc.name == "typed nil" {
				require.NotNil(t, reflect.TypeOf(dst.Data))
				require.True(t, reflect.ValueOf(dst.Data).IsNil())
			}
			if p, ok := src.Data.(*object); ok && p != nil {
				require.Equal(t, "secret", p.Secret)
			}
			if m, ok := src.Data.(map[string]any); ok {
				require.Equal(t, "secret", m["secret"])
			}
		})
	}
}

func TestIgnorePathsDynamicCycles(t *testing.T) {
	t.Parallel()
	type node struct {
		Next   any
		Secret string
		Name   string
	}
	p := &node{Secret: "secret", Name: "new"}
	p.Next = p
	m := map[string]any{"secret": "secret"}
	m["next"] = m
	s := make([]any, 1)
	s[0] = s
	for _, value := range []any{p, m, s} {
		src := struct{ Data any }{value}
		var dst struct{ Data any }
		require.NotPanics(t, func() {
			converter.Convert(src, &dst, converter.WithIgnorePaths("data.next.next.secret", "data.secret"))
		})
		require.NotNil(t, dst.Data)
	}
	require.Equal(t, "secret", p.Secret)
	require.Equal(t, "secret", m["secret"])
}

func TestIgnorePathsAliasedDestination(t *testing.T) {
	t.Parallel()
	type source struct{ Item *privateState }
	type destination struct{ Item *privateState }
	item := newPrivateState("secret", "name", 7)
	src, dst := source{&item}, destination{&item}
	converter.Convert(src, &dst, converter.WithIgnorePaths("item.secret"), converter.WithIgnoreFields("name"))
	require.NotSame(t, src.Item, dst.Item)
	require.Equal(t, "secret", src.Item.Secret)
	require.Equal(t, "name", dst.Item.Name)
	require.Equal(t, 7, dst.Item.State())
}

func TestIgnorePathsLeafAndCopySlices(t *testing.T) {
	t.Parallel()
	type item struct {
		Secret string
		Name   string
		Tags   []string
		state  int
	}
	type root struct{ Item item }
	src := root{item{"source secret", "source name", []string{"source tag"}, 7}}
	dst := root{Item: item{Secret: "keep secret", Name: "keep name"}}
	converter.Convert(src, &dst, converter.WithIgnorePaths("item.secret"), converter.WithIgnoreFields("name"), converter.WithCopySlices())
	require.Equal(t, "keep secret", dst.Item.Secret)
	require.Equal(t, "keep name", dst.Item.Name)
	require.Equal(t, 7, dst.Item.state)
	dst.Item.Tags[0] = "changed"
	require.Equal(t, "source tag", src.Item.Tags[0])
}

func TestIgnorePathsWithFieldMappingsPreservesDestination(t *testing.T) {
	t.Parallel()
	type item struct {
		Original string
		Target   string
		Secret   string
		state    int
	}
	source := item{"new", "unused", "source secret", 7}
	old := item{"keep original", "old target", "keep secret", 3}
	want := item{"keep original", "new", "keep secret", 7}
	opts := []converter.Option{
		converter.WithFieldMappings(map[string]string{"original": "target", "target": "missing"}),
		converter.WithIgnorePaths("item.secret"),
	}
	t.Run("static", func(t *testing.T) {
		src := struct{ Item item }{source}
		dst := struct{ Item item }{old}
		converter.Convert(src, &dst, opts...)
		require.Equal(t, want, dst.Item)
	})
	t.Run("dynamic", func(t *testing.T) {
		src := struct{ Item any }{source}
		dst := struct{ Item any }{old}
		converter.Convert(src, &dst, opts...)
		require.Equal(t, want, dst.Item)
	})
}

func TestConvertEWrappedFieldError(t *testing.T) {
	t.Parallel()
	fieldErr := coreerrs.NewFieldError("attrs.grade", "unknown value")
	wrapped := fmt.Errorf("enum conversion: %w", fieldErr)
	codec := func(_ string, _, _ reflect.Value, _ convcodec.CodecHandlerE) error {
		return wrapped
	}
	src, dst := struct{ Grade int }{1}, struct{ Grade int }{}
	_, err := converter.ConvertE(src, &dst, converter.WithErrorCodecs(codec))
	require.Same(t, wrapped, err)
}

func TestMixedCodecsKeepLegacyLeafName(t *testing.T) {
	t.Parallel()
	type sourceItem struct{ Grade int }
	type targetItem struct{ Grade string }
	src := struct{ Attrs sourceItem }{sourceItem{1}}
	var dst struct{ Attrs targetItem }
	var seenPath, seenName string
	errorCodec := func(path string, src, dst reflect.Value, next convcodec.CodecHandlerE) error {
		if src.Kind() == reflect.Int {
			seenPath = path
		}
		return next(path, src, dst)
	}
	legacyCodec := func(name string, src, dst reflect.Value, next convcodec.CodecHandler) {
		if src.Kind() == reflect.Int {
			seenName = name
			if name == "grade" {
				dst.SetString("junior")
				return
			}
		}
		next(name, src, dst)
	}
	converter.Convert(src, &dst, converter.WithErrorCodecs(errorCodec), converter.WithCodecs(legacyCodec))
	require.Equal(t, "attrs.grade", seenPath)
	require.Equal(t, "grade", seenName)
	require.Equal(t, "junior", dst.Attrs.Grade)
}

func TestDynamicIgnoreZeroPointer(t *testing.T) {
	type root struct{ Data any }
	zero, old := 0, 7
	src, dst := root{&zero}, root{&old}
	converter.Convert(src, &dst, converter.WithIgnoreZeroValues(), converter.WithIgnorePaths("data.secret"))
	require.Equal(t, 0, *dst.Data.(*int))
}

func TestDynamicNilInterfaceConcreteDestination(t *testing.T) {
	src := struct{ Data any }{}
	dst := struct{ Data int }{7}
	converter.Convert(src, &dst, converter.WithIgnorePaths("data.secret"))
	require.Equal(t, 7, dst.Data)
}

func TestDynamicNonEmptyInterface(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source any
		want   fmt.Stringer
	}{
		{"incompatible", 42, stringerValue("old")},
		{"assignable", stringerValue("new"), stringerValue("new")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := struct{ Data any }{tc.source}
			dst := struct{ Data fmt.Stringer }{stringerValue("old")}
			require.NotPanics(t, func() {
				converter.Convert(src, &dst, converter.WithIgnorePaths("data.secret"))
			})
			require.Equal(t, tc.want, dst.Data)
		})
	}
}

type stringerValue string

func (s stringerValue) String() string { return string(s) }

func TestErrorCodecsSkipMapKeys(t *testing.T) {
	var paths []string
	codec := func(path string, src, dst reflect.Value, next convcodec.CodecHandlerE) error {
		paths = append(paths, path)
		return next(path, src, dst)
	}
	src := struct{ Data map[bool]int }{map[bool]int{true: 3}}
	var dst struct{ Data map[string]int }
	converter.Convert(src, &dst, converter.WithErrorCodecs(codec))
	require.NotEmpty(t, paths)
	for _, path := range paths {
		require.NotEqual(t, "\x00key", path)
	}
	require.Equal(t, map[string]int{"": 3}, dst.Data)
}

func TestConvertECodecErrorIdentity(t *testing.T) {
	sentinel := fmt.Errorf("codec failure")
	codec := func(_ string, _, _ reflect.Value, _ convcodec.CodecHandlerE) error { return sentinel }
	src, dst := struct{ Data int }{1}, struct{ Data int }{}
	_, err := converter.ConvertE(src, &dst, converter.WithErrorCodecs(codec))
	require.Same(t, sentinel, err)
	_, isTypeError := err.(converter.CompileTimeTypeError)
	require.False(t, isTypeError)
}

func TestDynamicSparseMergeEmptyPointer(t *testing.T) {
	type object struct {
		Secret *string
		Name   *string
	}
	type root struct{ Data any }
	name := "old"
	old := &object{Name: &name}
	src, dst := root{&object{}}, root{old}
	converter.Convert(src, &dst, converter.WithSparseMerge(), converter.WithIgnorePaths("data.secret"))
	got, ok := dst.Data.(*object)
	require.True(t, ok, "interface must retain its pointer type")
	require.Nil(t, got)
	require.False(t, dst.Data == nil, "cleared pointer must remain a typed nil")
	require.Equal(t, "old", *old.Name)
}

func TestDynamicLegacyCodecRoot(t *testing.T) {
	for _, handle := range []bool{false, true} {
		t.Run(fmt.Sprintf("handle=%v", handle), func(t *testing.T) {
			calls := 0
			codec := func(name string, src, dst reflect.Value, next convcodec.CodecHandler) {
				if name == "data" {
					calls++
					if handle {
						dst.Set(reflect.ValueOf("handled"))
						return
					}
				}
				next(name, src, dst)
			}
			src := struct{ Data any }{42}
			var dst struct{ Data any }
			converter.Convert(src, &dst, converter.WithCodecs(codec), converter.WithIgnorePaths("data.secret"))
			require.Equal(t, 1, calls)
			if handle {
				require.Equal(t, "handled", dst.Data)
			} else {
				require.Equal(t, 42, dst.Data)
			}
		})
	}
}

func TestDynamicNestedCodecs(t *testing.T) {
	type object struct{ Secret, Name string }
	for _, tc := range []struct {
		name  string
		value any
		path  string
		want  any
	}{
		{"struct", struct{ Child any }{object{"secret", "new"}}, "data.child.secret", struct{ Child any }{object{"", "new"}}},
		{"map", map[string]any{"child": object{"secret", "new"}}, "data.child.secret", map[string]any{"child": object{"", "new"}}},
		{"slice", []any{object{"secret", "new"}}, "data.secret", []any{object{"", "new"}}},
		{"array", [1]any{object{"secret", "new"}}, "data.secret", [1]any{object{"", "new"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, handle := range []bool{false, true} {
				calls := 0
				codec := func(name string, src, dst reflect.Value, next convcodec.CodecHandler) {
					if src.Kind() == reflect.Interface {
						calls++
						if handle && src.Elem().Type() == reflect.TypeFor[object]() {
							dst.Set(reflect.ValueOf(object{Name: "handled"}))
							return
						}
					}
					next(name, src, dst)
				}
				src := struct{ Data any }{tc.value}
				var dst struct{ Data any }
				converter.Convert(src, &dst, converter.WithCodecs(codec), converter.WithIgnorePaths(tc.path))
				require.Equal(t, 2, calls)
				if !handle {
					require.Equal(t, tc.want, dst.Data)
				} else {
					require.Contains(t, fmt.Sprint(dst.Data), "handled")
				}
			}
		})
	}
}

func TestDynamicNestedErrorCodec(t *testing.T) {
	for _, failure := range []error{fmt.Errorf("nested codec failure"), coreerrs.NewFieldError("data.child", "invalid")} {
		var paths []string
		codec := func(path string, src, dst reflect.Value, next convcodec.CodecHandlerE) error {
			paths = append(paths, path)
			if path == "data.child" && src.Kind() == reflect.Interface {
				return failure
			}
			return next(path, src, dst)
		}
		src := struct{ Data any }{struct{ Child any }{42}}
		var dst struct{ Data any }
		_, err := converter.ConvertE(src, &dst, converter.WithErrorCodecs(codec), converter.WithIgnorePaths("data.child.secret"))
		require.True(t, err == failure, "got %v, want original error %v", err, failure)
		require.Equal(t, []string{"data", "data.child"}, paths)
	}
}
