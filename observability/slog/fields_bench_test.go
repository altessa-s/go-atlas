// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"testing"
)

func BenchmarkFields_Append(b *testing.B) {
	f := Fields{}
	b.ResetTimer()
	for b.Loop() {
		_ = f.Append("key", "value")
	}
}

func BenchmarkFields_ToSlogArgs(b *testing.B) {
	f := Fields{
		{Key: "a", Value: "1"},
		{Key: "b", Value: 2},
		{Key: "c", Value: true},
	}
	b.ResetTimer()
	for b.Loop() {
		f.ToSlogArgs()
	}
}

func BenchmarkFields_ToSlogAttrs(b *testing.B) {
	f := Fields{
		{Key: "a", Value: "1"},
		{Key: "b", Value: 2},
		{Key: "c", Value: true},
	}
	b.ResetTimer()
	for b.Loop() {
		f.ToSlogAttrs()
	}
}

func BenchmarkFieldsToAttrs_Flat(b *testing.B) {
	f := Fields{
		{Key: "a", Value: "1"},
		{Key: "b", Value: 2},
		{Key: "c", Value: true},
	}
	b.ResetTimer()
	for b.Loop() {
		FieldsToAttrs(f)
	}
}

func BenchmarkFieldsToAttrs_Nested(b *testing.B) {
	f := Fields{
		{Key: "http.method", Value: "GET"},
		{Key: "http.path", Value: "/api"},
		{Key: "db.system", Value: "postgres"},
	}
	b.ResetTimer()
	for b.Loop() {
		FieldsToAttrs(f)
	}
}

func BenchmarkInjectFields(b *testing.B) {
	f := Fields{
		{Key: "a", Value: "1"},
		{Key: "b", Value: 2},
	}
	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		InjectFields(ctx, f)
	}
}

func BenchmarkFieldsFromContext(b *testing.B) {
	f := Fields{{Key: "a", Value: "1"}, {Key: "b", Value: 2}}
	ctx := InjectFields(b.Context(), f)
	b.ResetTimer()
	for b.Loop() {
		FieldsFromContext(ctx)
	}
}
