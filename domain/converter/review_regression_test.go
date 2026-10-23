// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	"github.com/altessa-s/go-atlas/domain/converter"
	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

func TestReviewRootPrivateState(t *testing.T) {
	type item struct {
		Visible, Name string
		state         int
	}
	src, dst := item{"source", "new", 7}, item{"keep", "old", 3}
	converter.Convert(src, &dst, converter.WithIgnorePaths("visible"))
	if want := (item{"keep", "new", 3}); dst != want {
		t.Fatalf("root = %+v, want %+v", dst, want)
	}
	var out []item
	converter.Convert([]item{src}, &out, converter.WithIgnorePaths("visible"))
	if want := (item{"", "new", 7}); len(out) != 1 || out[0] != want {
		t.Fatalf("root slice = %+v, want [%+v]", out, want)
	}
}

func TestReviewMapKeySubtree(t *testing.T) {
	type protoGrade int32
	type domainGrade string
	type sourceKey struct{ Grade protoGrade }
	type targetKey struct{ Grade domainGrade }
	for _, ignored := range []bool{false, true} {
		t.Run(fmt.Sprint(ignored), func(t *testing.T) {
			var paths []string
			errorCodec := func(path string, src, dst reflect.Value, next convcodec.CodecHandlerE) error {
				paths = append(paths, path)
				if src.Type() == reflect.TypeFor[protoGrade]() {
					t.Errorf("map key reached error codec at %q", path)
				}
				return next(path, src, dst)
			}
			legacy := func(name string, src, dst reflect.Value, next convcodec.CodecHandler) {
				if src.Type() == reflect.TypeFor[protoGrade]() {
					dst.SetString("known")
					return
				}
				next(name, src, dst)
			}
			opts := []converter.Option{converter.WithErrorCodecs(errorCodec), converter.WithCodecs(legacy)}
			if ignored {
				opts = append(opts, converter.WithIgnorePaths("unused"))
			}
			src := struct{ Data map[sourceKey]int }{map[sourceKey]int{{1}: 42}}
			var dst struct{ Data map[targetKey]int }
			converter.Convert(src, &dst, opts...)
			for _, path := range paths {
				if strings.Contains(path, "\x00key") {
					t.Errorf("sentinel leaked: %q", path)
				}
			}
			if dst.Data[targetKey{"known"}] != 42 {
				t.Fatalf("converted map = %v", dst.Data)
			}
		})
	}
}

func TestReviewDynamicMapKeySentinelCollision(t *testing.T) {
	const key = "\x00key"
	for _, tc := range []struct {
		name   string
		value  any
		legacy bool
		want   any
	}{
		{name: "scalar", value: "source", want: "source"},
		{name: "nested", value: map[string]any{"value": "source"}, want: map[string]any{"value": "source"}},
		{name: "legacy", value: "source", legacy: true, want: "converted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			errorCodec := func(path string, src, dst reflect.Value, next convcodec.CodecHandlerE) error {
				paths = append(paths, path)
				return next(path, src, dst)
			}
			opts := []converter.Option{converter.WithErrorCodecs(errorCodec), converter.WithIgnorePaths("data.unused")}
			if tc.legacy {
				opts = append(opts, converter.WithCodecs(func(name string, src, dst reflect.Value, next convcodec.CodecHandler) {
					if name == key {
						dst.Set(reflect.ValueOf("converted"))
						return
					}
					next(name, src, dst)
				}))
			}
			var dst any
			for value := range converter.ConvertSeq[any, any]([]any{map[string]any{key: tc.value}}, opts...) {
				dst = value
			}
			if len(paths) == 0 {
				t.Fatal("error codec was never called")
			}
			for _, path := range paths {
				if path == key {
					t.Errorf("sentinel leaked: %q", path)
				}
			}
			if want := (map[string]any{key: tc.want}); !reflect.DeepEqual(dst, want) {
				t.Fatalf("converted map = %#v, want %#v", dst, want)
			}
		})
	}
}

func TestReviewCycleIgnoredDescendants(t *testing.T) {
	srcMap := map[string]any{"secret": "source"}
	srcMap["next"] = srcMap
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			var dst struct{ Data any }
			old := map[string]any{"name": "keep"}
			if existing {
				dst.Data = map[string]any{"next": old}
			}
			converter.Convert(struct{ Data any }{srcMap}, &dst, converter.WithIgnorePaths("data.next.secret"))
			back := dst.Data.(map[string]any)["next"].(map[string]any)
			if _, found := back["secret"]; found {
				t.Fatal("ignored secret leaked through back edge")
			}
			if reflect.ValueOf(back).Pointer() == reflect.ValueOf(srcMap).Pointer() {
				t.Fatal("back edge aliases source")
			}
			if existing && reflect.ValueOf(back).Pointer() != reflect.ValueOf(old).Pointer() {
				t.Fatal("existing back edge changed")
			}
		})
	}
	var dst struct{ Data any }
	converter.Convert(struct{ Data any }{srcMap}, &dst, converter.WithIgnorePaths("data.secret"))
	back := dst.Data.(map[string]any)["next"].(map[string]any)
	if reflect.ValueOf(back).Pointer() != reflect.ValueOf(srcMap).Pointer() {
		t.Fatal("unfiltered back edge must retain reference")
	}
}

func TestReviewSeqCodecErrorIdentity(t *testing.T) {
	sentinel := errors.New("codec failure")
	fieldErr := coreerrs.NewFieldError("grade", "unknown")
	for _, original := range []error{sentinel, fieldErr} {
		t.Run(original.Error(), func(t *testing.T) {
			defer func() {
				recovered := recover()
				err, ok := recovered.(error)
				if !ok || !errors.Is(err, original) {
					t.Fatalf("recovered %v does not wrap original %v", recovered, original)
				}
				if original == fieldErr {
					var got *coreerrs.FieldError
					if !errors.As(err, &got) || got != fieldErr {
						t.Fatalf("FieldError identity lost: %v", err)
					}
				}
			}()
			codec := func(_ string, _, _ reflect.Value, _ convcodec.CodecHandlerE) error { return original }
			conv := converter.New[int, string](converter.WithErrorCodecs(codec))
			for range conv.ConvertSeq([]int{1}) {
			}
			t.Fatal("expected codec panic")
		})
	}
}

func TestReviewDynamicSliceFreshElements(t *testing.T) {
	type item struct{ Secret, Name string }
	src := []item{{"source", "new"}}
	old := []item{{"keep", "old"}}
	static := struct{ Data []item }{old}
	dynamic := struct{ Data any }{old}
	opts := []converter.Option{converter.WithIgnorePaths("data.secret")}
	converter.Convert(struct{ Data []item }{src}, &static, opts...)
	converter.Convert(struct{ Data any }{src}, &dynamic, opts...)
	if static.Data[0].Secret != "" || !reflect.DeepEqual(dynamic.Data, static.Data) {
		t.Fatalf("dynamic = %v, static = %v", dynamic.Data, static.Data)
	}
	array := struct{ Data any }{[1]item{old[0]}}
	converter.Convert(struct{ Data any }{[1]item{src[0]}}, &array, opts...)
	if want := ([1]item{{"keep", "new"}}); array.Data != want {
		t.Fatalf("array = %v, want %v", array.Data, want)
	}
}

func TestReviewLegacyFieldErrorPanic(t *testing.T) {
	original := fmt.Errorf("legacy: %w", coreerrs.NewFieldError("grade", "unknown"))
	codec := func(_ string, _, _ reflect.Value, _ convcodec.CodecHandler) { panic(original) }
	var dst struct{ Grade string }
	_, err := converter.ConvertE(struct{ Grade int }{1}, &dst, converter.WithCodecs(codec))
	typeErr, ok := err.(converter.CompileTimeTypeError)
	if !ok || typeErr.Original != original {
		t.Fatalf("got %T: %v, want CompileTimeTypeError wrapping original", err, err)
	}
}
