// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validationconfig

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type testStruct struct {
	Name  string
	Value int
}

func TestValidateStruct_Valid(t *testing.T) {
	s := testStruct{Name: "test", Value: 42}
	err := ValidateStruct(&s,
		validation.Field(&s.Name, validation.Required),
	)
	require.NoError(t, err)
}

func TestValidateStruct_ValidationError(t *testing.T) {
	s := testStruct{Name: "", Value: 42}
	err := ValidateStruct(&s,
		validation.Field(&s.Name, validation.Required),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Name")
}

func TestValidateStruct_ErrFieldNotFound(t *testing.T) {
	s := testStruct{Name: "test"}
	other := testStruct{Name: "other"}

	err := ValidateStruct(&s,
		validation.Field(&other.Name, validation.Required),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "testStruct")
	assert.Contains(t, err.Error(), "field rule #0")
	assert.Contains(t, err.Error(), "pointer does not reference a field in the struct")
}

func TestValidateStruct_ErrFieldPointer(t *testing.T) {
	s := testStruct{Name: "test"}

	err := ValidateStruct(&s,
		validation.Field(s.Name, validation.Required),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "testStruct")
	assert.Contains(t, err.Error(), "field rule #0")
	assert.Contains(t, err.Error(), "field must be specified as a pointer")
}

func TestValidateStruct_NilStructPtr(t *testing.T) {
	s := testStruct{}
	err := ValidateStruct((*testStruct)(nil),
		validation.Field(&s.Name, validation.Required),
	)
	require.NoError(t, err)
}

func TestValidateStructIfEnabled_Disabled(t *testing.T) {
	s := testStruct{Name: ""}
	err := ValidateStructIfEnabled(false, &s,
		validation.Field(&s.Name, validation.Required),
	)
	require.NoError(t, err)
}

func TestValidateStructIfEnabled_Enabled(t *testing.T) {
	s := testStruct{Name: ""}
	err := ValidateStructIfEnabled(true, &s,
		validation.Field(&s.Name, validation.Required),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Name")
}

func TestValidateStructIfEnabled_ErrFieldNotFound(t *testing.T) {
	s := testStruct{Name: "test"}
	other := testStruct{Name: "other"}

	err := ValidateStructIfEnabled(true, &s,
		validation.Field(&other.Name, validation.Required),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "testStruct")
	assert.Contains(t, err.Error(), "pointer does not reference a field in the struct")
}

type nestedLeaf struct{ Name string }

func (l *nestedLeaf) Validate() error {
	return validation.ValidateStruct(l, validation.Field(&l.Name, validation.Required))
}

type nestedParent struct{ Leaf nestedLeaf }

func TestNestedField(t *testing.T) {
	t.Parallel()

	p := &nestedParent{}

	// The plain rule never reaches the pointer-receiver Validate.
	require.NoError(t, ValidateStruct(p, validation.Field(&p.Leaf)))

	err := ValidateStruct(p, NestedField(&p.Leaf))
	errs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "got %v", err)
	require.Contains(t, errs, "Leaf")

	p.Leaf.Name = "ok"
	require.NoError(t, ValidateStruct(p, NestedField(&p.Leaf)))
}
