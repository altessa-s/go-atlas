// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oneof_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter/codec/oneof"
)

// benchContractor is a representative payload struct.
type benchContractor struct {
	ID   int64
	Name string
}

// benchPayload mimics a domain struct with optional oneof variants.
type benchPayload struct {
	Contractor *benchContractor
	Agent      *benchContractor
}

// isBenchMessage_Payload mimics a protoc-generated oneof interface.
type isBenchMessage_Payload interface {
	isBenchMessage_Payload()
}

// benchMessage_Contractor mimics a protoc-generated oneof wrapper.
type benchMessage_Contractor struct {
	Contractor *benchContractor
}

func (*benchMessage_Contractor) isBenchMessage_Payload() {}

func BenchmarkNew_StructToOneof(b *testing.B) {
	codec := oneof.New(oneof.WithWrapperRegistry(map[string]any{
		"Contractor": &benchMessage_Contractor{},
	}))
	src := reflect.ValueOf(benchPayload{Contractor: &benchContractor{ID: 42, Name: "acme"}})
	var dst isBenchMessage_Payload
	dstVal := reflect.ValueOf(&dst).Elem()
	nop := func(_ string, _, _ reflect.Value) {}

	b.ReportAllocs()
	for b.Loop() {
		codec("Payload", src, dstVal, nop)
	}
}

func BenchmarkNewForField_NonTargetField(b *testing.B) {
	codec := oneof.NewForField("Payload")
	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()
	nop := func(_ string, _, _ reflect.Value) {}

	b.ReportAllocs()
	for b.Loop() {
		codec("Other", src, dstVal, nop)
	}
}
