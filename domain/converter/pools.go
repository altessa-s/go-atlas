// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	"reflect"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

const (
	// DefaultPoolCapacity is the default initial capacity for pooled slices
	// managed by [ObjectPools].
	DefaultPoolCapacity = 16
)

// ObjectPools manages all object pools for the converter.
// It is safe for concurrent use; each pool is backed by sync.Pool.
type ObjectPools struct {
	stringSlices *slices.Pool[string]
	boolSlices   *slices.Pool[bool]
	valueSlices  *slices.Pool[reflect.Value]
}

// NewObjectPools creates a new set of object pools for reducing allocations.
// The pools manage reusable slices to improve conversion performance
// by reducing garbage collection pressure.
//
// Example:
//
//	pools := NewObjectPools()
//	// Used internally by converter
func NewObjectPools() *ObjectPools {
	return &ObjectPools{
		stringSlices: slices.NewPool[string](DefaultPoolCapacity),
		boolSlices:   slices.NewPool[bool](DefaultPoolCapacity),
		valueSlices:  slices.NewPool[reflect.Value](DefaultPoolCapacity),
	}
}

// GetStringSlice gets a string slice from the pool with the given minimum
// capacity. It returns the pooled *[]string so [ObjectPools.PutStringSlice]
// receives the exact same pointer, preserving pool identity: a value-based API
// would let the same backing array be returned to the pool twice and then handed
// to two callers at once (a cross-caller aliasing hazard).
func (p *ObjectPools) GetStringSlice(minCap int) *[]string {
	if minCap <= 0 {
		minCap = DefaultPoolCapacity
	}
	return p.stringSlices.GetWithCapacity(minCap)
}

// PutStringSlice returns a string slice to the pool. slice must be the pointer
// obtained from [ObjectPools.GetStringSlice]; after the call it must not be used.
func (p *ObjectPools) PutStringSlice(slice *[]string) {
	if slice != nil {
		p.stringSlices.Put(slice)
	}
}

// GetBoolSlice gets a bool slice of the given length (cleared) from the pool.
// It returns the pooled pointer; see [ObjectPools.GetStringSlice] for why.
func (p *ObjectPools) GetBoolSlice(length int) *[]bool {
	slicePtr := p.boolSlices.GetWithCapacity(length)
	if length <= 0 {
		return slicePtr
	}
	if cap(*slicePtr) < length {
		*slicePtr = make([]bool, length)
	} else {
		*slicePtr = (*slicePtr)[:length]
		clear(*slicePtr)
	}
	return slicePtr
}

// PutBoolSlice returns a bool slice to the pool. slice must be the pointer
// obtained from [ObjectPools.GetBoolSlice]; after the call it must not be used.
func (p *ObjectPools) PutBoolSlice(slice *[]bool) {
	if slice != nil {
		p.boolSlices.Put(slice)
	}
}

// GetValueSlice gets a reflect.Value slice of the given length (cleared) from the
// pool. It returns the pooled pointer; see [ObjectPools.GetStringSlice] for why.
func (p *ObjectPools) GetValueSlice(length int) *[]reflect.Value {
	slicePtr := p.valueSlices.GetWithCapacity(length)
	if length <= 0 {
		return slicePtr
	}
	if cap(*slicePtr) < length {
		*slicePtr = make([]reflect.Value, length)
	} else {
		*slicePtr = (*slicePtr)[:length]
		clear(*slicePtr)
	}
	return slicePtr
}

// PutValueSlice returns a reflect.Value slice to the pool. slice must be the
// pointer obtained from [ObjectPools.GetValueSlice]; after the call it must not
// be used.
func (p *ObjectPools) PutValueSlice(slice *[]reflect.Value) {
	if slice != nil {
		p.valueSlices.Put(slice)
	}
}

// Global pools instance for package-level convenience functions
var globalPools = NewObjectPools()

// Package-level convenience functions that use the global pools

// getPooledBoolSlice gets a bool slice from the global pool
func getPooledBoolSlice(length int) *[]bool {
	return globalPools.GetBoolSlice(length)
}

// getPooledValueSlice gets a reflect.Value slice from the global pool
func getPooledValueSlice(length int) *[]reflect.Value {
	return globalPools.GetValueSlice(length)
}

// putPooledValueSlice returns a reflect.Value slice to the global pool
func putPooledValueSlice(slice *[]reflect.Value) {
	globalPools.PutValueSlice(slice)
}

// putPooledBoolSlice returns a bool slice to the global pool
func putPooledBoolSlice(slice *[]bool) {
	globalPools.PutBoolSlice(slice)
}
