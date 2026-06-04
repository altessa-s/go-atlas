// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

func BenchmarkMarshalBSONValue_SomeTime(b *testing.B) {
	opt := optional.Some(time.Now().UTC())
	var (
		typ  byte
		data []byte
		err  error
	)
	for b.Loop() {
		typ, data, err = opt.MarshalBSONValue()
	}
	_, _, _ = typ, data, err
}

func BenchmarkMarshalBSONValue_None(b *testing.B) {
	opt := optional.None[time.Time]()
	var (
		typ  byte
		data []byte
		err  error
	)
	for b.Loop() {
		typ, data, err = opt.MarshalBSONValue()
	}
	_, _, _ = typ, data, err
}

func BenchmarkUnmarshalBSONValue_SomeTime(b *testing.B) {
	typ, data, err := bson.MarshalValue(time.Now().UTC())
	if err != nil {
		b.Fatal(err)
	}
	rawType := byte(typ)
	var opt optional.Optional[time.Time]
	for b.Loop() {
		if err = opt.UnmarshalBSONValue(rawType, data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalBSONValue_Null(b *testing.B) {
	var opt optional.Optional[time.Time]
	for b.Loop() {
		if err := opt.UnmarshalBSONValue(byte(bson.TypeNull), nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalJSON_Some(b *testing.B) {
	opt := optional.Some("hello")
	var (
		raw []byte
		err error
	)
	for b.Loop() {
		raw, err = opt.MarshalJSON()
	}
	_, _ = raw, err
}

func BenchmarkMarshalJSON_None(b *testing.B) {
	opt := optional.None[string]()
	var (
		raw []byte
		err error
	)
	for b.Loop() {
		raw, err = opt.MarshalJSON()
	}
	_, _ = raw, err
}

func BenchmarkUnmarshalJSON_Some(b *testing.B) {
	in := []byte(`"hello"`)
	var opt optional.Optional[string]
	for b.Loop() {
		if err := opt.UnmarshalJSON(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalJSON_Null(b *testing.B) {
	in := []byte("null")
	var opt optional.Optional[string]
	for b.Loop() {
		if err := opt.UnmarshalJSON(in); err != nil {
			b.Fatal(err)
		}
	}
}
