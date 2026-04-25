// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oneof_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/domain/converter/codec/oneof"
)

func TestNew_PassThrough(t *testing.T) {
	codec := oneof.New()

	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Name", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "next should be called for non-oneof types")
}

func TestNewForField_PassThrough(t *testing.T) {
	codec := oneof.NewForField("Payload")

	src := reflect.ValueOf("hello")
	var dst string
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("OtherField", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "next should be called for non-target fields")
}
