// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package unixtime

import (
	"reflect"
	"testing"
	"time"
)

func BenchmarkNew_TimeToInt64(b *testing.B) {
	codec := New()
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	src := reflect.ValueOf(ts)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst int64
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("CreatedAt", src, dstVal, handler)
	}
}

func BenchmarkNew_Int64ToTime(b *testing.B) {
	codec := New()
	var unix int64 = 1750000000
	src := reflect.ValueOf(unix)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst time.Time
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("CreatedAt", src, dstVal, handler)
	}
}

func BenchmarkNew_PassThrough(b *testing.B) {
	codec := New()
	src := reflect.ValueOf("hello")
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst string
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("Name", src, dstVal, handler)
	}
}
