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

// The fixtures below mirror the exact shape protoc-gen-go emits for a oneof:
// an unexported single-method interface named "is{Message}_{Field}" and
// wrapper structs whose marker method is unexported with a pointer receiver.
// reflect cannot see that marker, which is what the regression tests pin.

type testContractor struct {
	Name string
}

type testAgent struct {
	Name string
}

type isTestMessage_Payload interface { //nolint:revive // protoc naming mirrored on purpose
	isTestMessage_Payload()
}

type TestMessage_Contractor struct { //nolint:revive // protoc naming mirrored on purpose
	Contractor *testContractor
}

func (*TestMessage_Contractor) isTestMessage_Payload() {}

type TestMessage_Agent struct { //nolint:revive // protoc naming mirrored on purpose
	Agent *testAgent
}

func (*TestMessage_Agent) isTestMessage_Payload() {}

type testMessage struct {
	Payload isTestMessage_Payload
}

type testPayload struct {
	Contractor *testContractor
	Agent      *testAgent
}

// TestNew_OneofToStruct_InterfaceField is the regression test for the
// unreachable oneof→struct direction: the codec receives the oneof interface
// field verbatim (as the parent converter dispatches it) and must unpack the
// active wrapper into the destination struct.
func TestNew_OneofToStruct_InterfaceField(t *testing.T) {
	codec := oneof.New()

	msg := testMessage{Payload: &TestMessage_Contractor{Contractor: &testContractor{Name: "acme"}}}
	src := reflect.ValueOf(msg).FieldByName("Payload")

	var dst testPayload
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Payload", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.False(t, nextCalled, "codec must handle the oneof interface itself")
	if assert.NotNil(t, dst.Contractor, "active wrapper field must be unpacked") {
		assert.Equal(t, "acme", dst.Contractor.Name)
	}
	assert.Nil(t, dst.Agent, "inactive oneof variants must stay nil")
}

// TestNew_OneofToStruct_ConcreteWrapper covers the already-unwrapped shape:
// the source is the wrapper itself, recognized through the wrapper registry.
func TestNew_OneofToStruct_ConcreteWrapper(t *testing.T) {
	codec := oneof.New(
		oneof.WithWrapperRegistry(map[string]any{
			"Contractor": &TestMessage_Contractor{},
			"Agent":      &TestMessage_Agent{},
		}),
	)

	src := reflect.ValueOf(&TestMessage_Agent{Agent: &testAgent{Name: "bond"}})

	var dst testPayload
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Payload", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.False(t, nextCalled, "registered wrapper must be handled by the codec")
	if assert.NotNil(t, dst.Agent) {
		assert.Equal(t, "bond", dst.Agent.Name)
	}
	assert.Nil(t, dst.Contractor)
}

// TestNew_OneofToStruct_NilInterface pins the nil-oneof behavior for both
// option modes: ignored when WithIgnoreNilFields is set, passed to the next
// handler otherwise.
func TestNew_OneofToStruct_NilInterface(t *testing.T) {
	msg := testMessage{}
	src := reflect.ValueOf(msg).FieldByName("Payload")

	var dst testPayload
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	oneof.New(oneof.WithIgnoreNilFields())("Payload", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})
	assert.False(t, nextCalled, "nil oneof must be swallowed with WithIgnoreNilFields")

	nextCalled = false
	oneof.New()("Payload", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})
	assert.True(t, nextCalled, "nil oneof must fall through without WithIgnoreNilFields")
}

// TestNew_OneofToStruct_UnregisteredConcreteWrapper pins the conservative
// side: a bare struct that merely looks wrapper-ish must not be treated as a
// oneof without registry evidence.
func TestNew_OneofToStruct_UnregisteredConcreteWrapper(t *testing.T) {
	codec := oneof.New()

	src := reflect.ValueOf(&TestMessage_Agent{Agent: &testAgent{Name: "bond"}})

	var dst testPayload
	dstVal := reflect.ValueOf(&dst).Elem()

	nextCalled := false
	codec("Payload", src, dstVal, func(_ string, _, _ reflect.Value) {
		nextCalled = true
	})

	assert.True(t, nextCalled, "unregistered concrete wrapper must fall through to next")
	assert.Nil(t, dst.Agent)
}

// TestNew_StructToOneof_RoundTrip closes the loop: struct→oneof via the
// registry, then oneof→struct back through the interface detection.
func TestNew_StructToOneof_RoundTrip(t *testing.T) {
	codec := oneof.New(
		oneof.WithWrapperRegistry(map[string]any{
			"Contractor": &TestMessage_Contractor{},
			"Agent":      &TestMessage_Agent{},
		}),
	)

	payload := testPayload{Contractor: &testContractor{Name: "acme"}}
	var msg testMessage

	srcField := reflect.ValueOf(payload)
	dstField := reflect.ValueOf(&msg).Elem().FieldByName("Payload")

	codec("Payload", srcField, dstField, func(_ string, _, _ reflect.Value) {
		t.Fatal("struct→oneof must be handled by the codec")
	})

	wrapper, ok := msg.Payload.(*TestMessage_Contractor)
	if assert.True(t, ok, "payload must hold the contractor wrapper") {
		assert.Equal(t, "acme", wrapper.Contractor.Name)
	}

	var back testPayload
	backVal := reflect.ValueOf(&back).Elem()
	codec("Payload", reflect.ValueOf(msg).FieldByName("Payload"), backVal, func(_ string, _, _ reflect.Value) {
		t.Fatal("oneof→struct must be handled by the codec")
	})

	if assert.NotNil(t, back.Contractor) {
		assert.Equal(t, "acme", back.Contractor.Name)
	}
}
