// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tspb_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/domain/converter/codec/tspb"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func BenchmarkNew_TimestampToTime(b *testing.B) {
	codec := tspb.New()
	ts := timestamppb.New(time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))
	src := reflect.ValueOf(ts).Elem()
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst time.Time
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("CreatedAt", src, dstVal, handler)
	}
}

func BenchmarkNew_TimeToTimestamp(b *testing.B) {
	codec := tspb.New()
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	src := reflect.ValueOf(now)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst timestamppb.Timestamp
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("CreatedAt", src, dstVal, handler)
	}
}

func BenchmarkNew_TimestampToInt64(b *testing.B) {
	codec := tspb.New()
	ts := timestamppb.New(time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))
	src := reflect.ValueOf(ts).Elem()
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst int64
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("CreatedAt", src, dstVal, handler)
	}
}

func BenchmarkNew_Int64ToTimestamp(b *testing.B) {
	codec := tspb.New()
	var unix int64 = 1750000000
	src := reflect.ValueOf(unix)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst timestamppb.Timestamp
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("CreatedAt", src, dstVal, handler)
	}
}

func BenchmarkNew_PassThrough(b *testing.B) {
	codec := tspb.New()
	src := reflect.ValueOf("hello")
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		var dst string
		dstVal := reflect.ValueOf(&dst).Elem()
		codec("Name", src, dstVal, handler)
	}
}
