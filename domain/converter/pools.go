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
	typeSlices   *slices.Pool[reflect.Type]
	kindSlices   *slices.Pool[reflect.Kind]
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
		typeSlices:   slices.NewPool[reflect.Type](DefaultPoolCapacity),
		kindSlices:   slices.NewPool[reflect.Kind](DefaultPoolCapacity),
	}
}

// GetStringSlice gets a string slice from the pool with minimum capacity
func (p *ObjectPools) GetStringSlice(minCap int) []string {
	if minCap <= 0 {
		minCap = DefaultPoolCapacity
	}
	slicePtr := p.stringSlices.GetWithCapacity(minCap)
	return *slicePtr
}

// PutStringSlice returns a string slice to the pool
func (p *ObjectPools) PutStringSlice(slice []string) {
	if slice != nil {
		slice = slice[:0] // Reset length
		p.stringSlices.Put(&slice)
	}
}

// GetBoolSlice gets a bool slice from the pool with minimum length
func (p *ObjectPools) GetBoolSlice(length int) []bool {
	if length <= 0 {
		return []bool{}
	}

	slicePtr := p.boolSlices.GetWithCapacity(length)
	slice := *slicePtr

	// Ensure we have enough capacity
	if cap(slice) < length {
		slice = make([]bool, length)
	} else {
		slice = slice[:length]
		// Clear the slice values
		for i := range slice {
			slice[i] = false
		}
	}

	return slice
}

// PutBoolSlice returns a bool slice to the pool
func (p *ObjectPools) PutBoolSlice(slice []bool) {
	if slice != nil {
		slice = slice[:0] // Reset length
		p.boolSlices.Put(&slice)
	}
}

// GetValueSlice gets a reflect.Value slice from the pool
func (p *ObjectPools) GetValueSlice(length int) []reflect.Value {
	if length <= 0 {
		return []reflect.Value{}
	}

	slicePtr := p.valueSlices.GetWithCapacity(length)
	slice := *slicePtr

	// Ensure we have enough capacity
	if cap(slice) < length {
		slice = make([]reflect.Value, length)
	} else {
		slice = slice[:length]
		// Clear the slice values
		for i := range slice {
			slice[i] = reflect.Value{}
		}
	}

	return slice
}

// PutValueSlice returns a reflect.Value slice to the pool
func (p *ObjectPools) PutValueSlice(slice []reflect.Value) {
	if slice != nil {
		slice = slice[:0] // Reset length
		p.valueSlices.Put(&slice)
	}
}

// Global pools instance for package-level convenience functions
var globalPools = NewObjectPools()

// Package-level convenience functions that use the global pools

// getPooledBoolSlice gets a bool slice from the global pool
func getPooledBoolSlice(length int) []bool {
	return globalPools.GetBoolSlice(length)
}

// getPooledValueSlice gets a reflect.Value slice from the global pool
func getPooledValueSlice(length int) []reflect.Value {
	return globalPools.GetValueSlice(length)
}

// putPooledValueSlice returns a reflect.Value slice to the global pool
func putPooledValueSlice(slice []reflect.Value) {
	globalPools.PutValueSlice(slice)
}

// putPooledBoolSlice returns a bool slice to the global pool
func putPooledBoolSlice(slice []bool) {
	globalPools.PutBoolSlice(slice)
}
