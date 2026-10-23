// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/domain/converter"
	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

func TestAssignablePaths(t *testing.T) {
	for _, codecs := range []bool{false, true} {
		name := "direct"
		if codecs {
			name = "after codecs"
		}
		t.Run(name, func(t *testing.T) {
			opts := []converter.Option{converter.WithCopySlices()}
			if codecs {
				opts = append(opts, converter.WithErrorCodecs(func(p string, s, d reflect.Value, next convcodec.CodecHandlerE) error { return next(p, s, d) }))
			}
			t.Run("pointer slice", func(t *testing.T) {
				items := []int{1, 2}
				src := struct{ Items *[]int }{&items}
				var dst struct{ Items *[]int }
				converter.Convert(src, &dst, opts...)
				if dst.Items == src.Items || !reflect.DeepEqual(*dst.Items, items) {
					t.Fatalf("slice pointer was not copied: %+v", dst)
				}
				(*dst.Items)[0] = 9
				if items[0] != 1 {
					t.Fatal("slice storage aliases source")
				}
			})
			t.Run("pointer slice to interface", func(t *testing.T) {
				items := []int{1, 2}
				src := struct{ Items *[]int }{&items}
				var dst struct{ Items any }
				converter.Convert(src, &dst, opts...)
				got, ok := dst.Items.(*[]int)
				if !ok || got == nil {
					t.Fatalf("dst.Items = %v (%T), want non-nil *[]int", dst.Items, dst.Items)
				}
				if got == src.Items {
					t.Fatal("slice pointer aliases source")
				}
				if !reflect.DeepEqual(*got, items) {
					t.Fatalf("slice contents = %v, want %v", *got, items)
				}
				(*got)[0] = 9
				if items[0] != 1 {
					t.Fatal("slice storage aliases source")
				}
			})
			t.Run("nil containers", func(t *testing.T) {
				type item struct{ Secret string }
				type root struct {
					Items []item
					Map   map[string]item
				}
				src := root{}
				dst := root{[]item{{"old"}}, map[string]item{"k": {"old"}}}
				converter.Convert(src, &dst, append(opts, converter.WithIgnorePaths("items.secret", "map.secret"))...)
				if dst.Items != nil || dst.Map != nil {
					t.Fatalf("nil containers = %+v", dst)
				}
			})
			t.Run("dynamic ignore", func(t *testing.T) {
				type item struct{ Secret, Name string }
				type root struct{ Data any }
				src := root{item{"source", "new"}}
				dst := root{item{"keep", "old"}}
				converter.Convert(src, &dst, append(opts, converter.WithIgnorePaths("data.secret"))...)
				if dst.Data != (item{"keep", "new"}) {
					t.Fatalf("dynamic = %+v", dst.Data)
				}
			})
			t.Run("sparse and opaque", func(t *testing.T) {
				type item struct {
					Keep   *string
					Change string
				}
				type root struct {
					Item item
					Time time.Time
				}
				keep := "keep"
				src := root{item{nil, "new"}, time.Unix(123, 0)}
				dst := root{item{&keep, "old"}, time.Unix(456, 0)}
				converter.Convert(src, &dst, append(opts, converter.WithSparseMerge())...)
				if dst.Item != (item{&keep, "new"}) || !dst.Time.Equal(src.Time) {
					t.Fatalf("sparse = %+v", dst)
				}
			})
		})
	}
}

func TestDynamicFieldAndMapKeyEquivalence(t *testing.T) {
	type object struct{ Secret, Alias, Name, Skip string }
	type root struct{ Data any }
	for _, data := range []any{object{Secret: "hidden", Name: "new", Skip: "ignored"}, map[string]any{"Secret": "hidden", "Name": "new", "Skip": "ignored"}} {
		var paths []string
		dst := root{}
		switch data.(type) {
		case object:
			dst.Data = object{Secret: "keep", Skip: "old"}
		default:
			dst.Data = map[string]any{"Secret": "keep", "Missing": "keep missing", "Skip": "old"}
		}
		_, err := converter.ConvertE(root{data}, &dst,
			converter.WithIgnorePaths("data.secret", "data.missing"),
			converter.WithIgnoreFields("skip"),
			converter.WithFieldMappings(map[string]string{"name": "alias"}),
			converter.WithErrorCodecs(func(p string, s, d reflect.Value, next convcodec.CodecHandlerE) error {
				paths = append(paths, p)
				return next(p, s, d)
			}))
		if err != nil {
			t.Fatal(err)
		}
		switch got := dst.Data.(type) {
		case object:
			if got != (object{Secret: "keep", Alias: "new", Skip: "old"}) {
				t.Fatalf("struct = %+v", got)
			}
		case map[string]any:
			want := map[string]any{"Secret": "keep", "Missing": "keep missing", "Name": "new", "Skip": "old"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("map = %+v", got)
			}
		}
		found := false
		for _, p := range paths {
			if p == "data.name" {
				found = true
			}
			if p == "data.secret" || p == "data.skip" {
				t.Fatalf("ignored path reached codec: %q", p)
			}
		}
		if !found {
			t.Fatalf("source name path absent: %v", paths)
		}
	}
}
