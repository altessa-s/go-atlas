// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"log/slog"
	"reflect"
	"unsafe"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Value represents a secret value container with metadata and security features.
// It encapsulates both the original and encoded forms of secret data along with
// versioning information for change tracking. The Value type provides secure
// memory management through automatic cleanup and finalizer support.
//
// The Value container supports generic types, allowing storage of different
// secret value types such as strings, byte arrays, or custom structs. It
// maintains both the original typed value and its encoded representation
// for storage provider compatibility.
//
// Security Features:
// Value instances include automatic memory clearing capabilities through the
// Clear() method and runtime finalizers. This ensures sensitive data is
// properly removed from memory when the Value is no longer needed.
//
// Thread Safety:
// Individual Value instances are not thread-safe and should not be modified
// concurrently. However, the Clear() method is safe to call multiple times
// and from different goroutines.
//
// JSON Serialization:
// Only non-sensitive metadata (Key, EncodedKey, Version) is JSON-serialized;
// the secret payload fields (Value, EncodedValue) carry json:"-" so an
// accidental json.Marshal or slog.Any never leaks the secret. Persistence goes
// through the codec, which encodes the payload directly rather than marshaling
// this wrapper. Value also implements [slog.LogValuer] to redact itself in logs.
type Value[T any] struct {
	// Key is the original unencoded secret identifier used by clients for lookups
	Key string `json:"key"`

	// EncodedKey is the storage-ready encoded form of the key used by providers
	EncodedKey string `json:"encoded_key"`

	// Value contains the actual secret data of type T
	// Common types include string, []byte, or custom configuration structs.
	// It is never JSON-serialized (json:"-") so an accidental json.Marshal or
	// response encode can never emit the plaintext secret.
	Value T `json:"-"`

	// EncodedValue is the encoded binary representation of the secret value.
	// Like Value it is excluded from JSON (json:"-"); storage providers persist
	// the encoded bytes through the codec, not by marshaling this wrapper.
	EncodedValue []byte `json:"-"`

	// Version is the provider-specific version identifier for change tracking
	// Format varies by provider (e.g., sequential numbers, timestamps, hashes)
	Version string `json:"version"`

	ss      *corestrings.SecureString
	cleanup coreruntime.Cleanup
}

const (
	defaultSliceCapacity = 64   // Default capacity for pooled slices
	maxPoolCapacity      = 1024 // Maximum capacity to store in pool
)

// LogValue implements [slog.LogValuer] so a Value is redacted whenever it reaches
// a structured logger (e.g. via slog.Any). It exposes only non-sensitive metadata
// and never the secret payload. slog treats a LogValuer atomically, so the secret
// fields are not walked by reflection.
func (v *Value[T]) LogValue() slog.Value {
	if v == nil {
		return slog.StringValue("<nil secret>")
	}
	return slog.GroupValue(
		slog.String("key", v.Key),
		slog.String("version", v.Version),
		slog.String("value", "[REDACTED]"),
	)
}

// Clear securely clears all sensitive data from the Value and removes any finalizer.
// This method zeros out the EncodedValue slice and attempts to clear
// the Value field if it contains sensitive data. It's safe to call
// multiple times.
//
// For maximum security, this method should be called when the secret
// is no longer needed to prevent sensitive data from remaining in memory
// until garbage collection.
// multiple goroutines.
func (v *Value[T]) Clear() {
	// Stop and remove cleanup first to prevent unnecessary overhead
	if v.cleanup != nil {
		v.cleanup.Stop()
		v.cleanup = nil
	}

	// If we have a SecureString, clear it
	if v.ss != nil {
		v.ss.Clear()
		v.ss = nil
	}

	// Securely clear the encoded value slice using optimized utility
	if v.EncodedValue != nil {
		corestrings.ZeroBytes(v.EncodedValue)
		v.EncodedValue = nil
	}

	// Attempt to clear the Value field if it contains sensitive data
	v.clearValueField()

	// Key / EncodedKey / Version are identifiers and metadata, not
	// secret material — the actual secret bytes live in EncodedValue
	// (zeroed by clearValueField above) and the typed Value field.
	// Previously we also ran ZeroString over these fields, but that
	// fires SIGSEGV + debug.SetPanicOnFault recovery for every
	// interned string (and string literals returned by providers are
	// commonly interned). The fault-and-recover is functional but
	// expensive enough to surface in tracing under load. Drop the
	// unsafe zeroing for identifier fields and just reset the
	// references — the underlying bytes are unreachable from this
	// struct after the assignment and will be GC'd normally.
	v.Key = ""
	v.EncodedKey = ""
	v.Version = ""
}

// clearValueField attempts to securely clear the Value field based on its type
func (v *Value[T]) clearValueField() {
	valuePtr := &v.Value
	val := reflect.ValueOf(valuePtr).Elem()

	switch val.Kind() {
	case reflect.String:
		// Attempt to zero the string memory if possible
		s := val.String()
		if s != "" {
			corestrings.ZeroString(s)
			if val.CanSet() {
				val.SetString("")
			}
		}

	case reflect.Slice:
		if val.Type().Elem().Kind() == reflect.Uint8 { // []byte
			// Zero out byte slices
			if slice, ok := val.Interface().([]byte); ok && slice != nil {
				corestrings.ZeroBytes(slice)
			}
			if val.CanSet() {
				val.Set(reflect.Zero(val.Type()))
			}
		} else if val.CanSet() {
			// For other slice types, set to nil
			val.Set(reflect.Zero(val.Type()))
		}

	case reflect.Map:
		// Clear maps by setting to nil
		if val.CanSet() {
			val.Set(reflect.Zero(val.Type()))
		}

	case reflect.Ptr:
		// For pointer types, set to nil
		if val.CanSet() {
			val.Set(reflect.Zero(val.Type()))
		}

	case reflect.Interface:
		// For interface types, set to nil
		if val.CanSet() {
			val.Set(reflect.Zero(val.Type()))
		}

	default:
		// For other types, set to zero value
		if val.CanSet() {
			val.Set(reflect.Zero(val.Type()))
		}
	}
}

type cleanupData struct {
	encodedValue []byte
	ss           *corestrings.SecureString
	// We can't safely clear Value T via AddCleanup as it creates retention cycles
	// Key, EncodedKey, Version are strings, capturing them captures their backing array pointers which is safe
	key        string
	encodedKey string
	version    string
}

// NewValue creates a new Value with automatic cleanup via finalizer.
// The finalizer ensures that sensitive data is cleared when the Value is
// garbage collected, providing an additional layer of security.
//
// Note: Finalizers are not guaranteed to run, so explicit Clear() calls
// are still recommended for maximum security.
func NewValue[T any](key string, value T, encodedValue []byte, version string) *Value[T] {
	v := &Value[T]{
		Key:          key,
		Value:        value,
		EncodedValue: encodedValue,
		Version:      version,
	}

	// If T is a string, initialize SecureString for better memory management
	if reflect.TypeFor[T]().Kind() == reflect.String {
		v.ss = corestrings.NewSecureString(any(value).(string)) //nolint:errcheck // type checked above
	}

	// Prepare independent cleanup data to avoid capturing 'v' in the cleanup argument
	// capturing 'v' would prevent garbage collection or cause runtime panic with AddCleanup
	data := &cleanupData{
		encodedValue: encodedValue,
		ss:           v.ss,
		key:          key,
		encodedKey:   "", // NewValue doesn't set EncodedKey
		version:      version,
	}

	// Set cleanup to automatically clear sensitive data using Go 1.24+ runtime mechanism.
	// Same rationale as [Value.Clear]: only the secret-bearing fields
	// (encodedValue bytes, the SecureString-wrapped typed value) get
	// zeroed. Key / EncodedKey / Version are identifiers/metadata —
	// running ZeroString over them costs a SIGSEGV+recover per call
	// for interned strings without protecting anything sensitive.
	v.cleanup = coreruntime.AddCleanup(v, func(d *cleanupData) {
		if d.encodedValue != nil {
			corestrings.ZeroBytes(d.encodedValue)
		}
		if d.ss != nil {
			d.ss.Clear()
		}
	}, data)

	return v
}

// valueSlicePool is a shared pool for reusing Value slice buffers to reduce allocations.
// Uses the core/collections/slices.Pool implementation for generic slice pooling.
var valueSlicePool = coreslices.NewPool[*Value[any]](defaultSliceCapacity)

// GetValueSlice retrieves a slice buffer from the pool and resets it.
// This function is used by providers to reduce memory allocations during concurrent operations.
//
// Returns a slice pointer that should be returned to the pool using PutValueSlice.
//
// # Safety of Unsafe Type Conversion
//
// The conversion `(*[]*Value[T])(unsafe.Pointer(slice))` is safe because:
//
//  1. Memory Layout Equivalence: []*Value[T] and []*Value[any] have identical memory layouts.
//     Go slices are represented as {data pointer, length, capacity}. The element type affects
//     only how the compiler interprets the data pointer, not the slice header itself.
//
//  2. Pointer-Only Elements: Both slice types contain pointers (*Value[T] and *Value[any]).
//     All pointers have the same size and alignment regardless of the pointed-to type.
//
//  3. Generic Type Erasure: At runtime, *Value[T] and *Value[any] are both just pointers.
//     The generic type parameter T only affects compile-time type checking, not runtime
//     memory layout. The Value struct layout is identical for all T due to Go's uniform
//     representation of interface{}/any fields.
//
//  4. Pool Isolation: The slice is reset to zero length before return, ensuring no stale
//     typed pointers leak between different T instantiations.
//
// Performance: This enables a single sync.Pool to serve all generic instantiations,
// reducing memory overhead and improving cache efficiency.
func GetValueSlice[T any]() *[]*Value[T] {
	slice := valueSlicePool.Get()
	// Safe type conversion - see function documentation for safety invariants
	// #nosec G103 -- intentional type conversion with identical memory layouts
	return (*[]*Value[T])(unsafe.Pointer(slice))
}

// GetValueSliceWithCapacity retrieves a slice buffer from the pool with capacity hint.
// This function allows providers to optimize memory allocation by providing a capacity hint.
//
// Parameters:
//   - expectedCapacity: hint about the expected number of elements to reduce reallocations
//
// Returns a slice pointer that should be returned to the pool using PutValueSlice.
//
// # Safety
//
// See GetValueSlice documentation for unsafe pointer conversion safety invariants.
func GetValueSliceWithCapacity[T any](expectedCapacity int) *[]*Value[T] {
	slice := valueSlicePool.GetWithCapacity(expectedCapacity)
	// Safe type conversion - see GetValueSlice documentation for safety invariants
	// #nosec G103 -- intentional type conversion with identical memory layouts
	return (*[]*Value[T])(unsafe.Pointer(slice))
}

// PutValueSlice returns a slice buffer to the pool for reuse.
// This function should be called when providers are done with the slice to return it to the pool.
// It also securely clears any Value instances before returning to the pool.
//
// # Safety
//
// The conversion `(*[]*Value[any])(unsafe.Pointer(slice))` is safe for the same reasons
// as GetValueSlice (see its documentation). Additionally:
//
//  1. Pre-Clear: All Value instances are cleared before conversion, ensuring no typed
//     data remains in the slice when it's returned to the pool.
//
//  2. Zero Length: The slice is reset to zero length, so no stale pointers are accessible
//     even if the slice is retrieved by a different generic instantiation.
//
//  3. Pool Type Safety: The pool stores *[]*Value[any], and all generic instantiations
//     convert to/from this common type, maintaining consistency.
func PutValueSlice[T any](slice *[]*Value[T]) {
	if slice == nil {
		return
	}

	// Securely clear the slice contents and clear sensitive data
	// This must happen regardless of whether we pool the slice
	for i := range *slice {
		if (*slice)[i] != nil {
			// Clear sensitive data from the Value before releasing reference
			(*slice)[i].Clear()
		}
	}

	// Don't pool overly large slices to avoid memory waste
	if cap(*slice) > maxPoolCapacity {
		return
	}

	// Safe type conversion - see GetValueSlice documentation for safety invariants
	// #nosec G103 -- intentional type conversion with identical memory layouts
	anySlice := (*[]*Value[any])(unsafe.Pointer(slice))
	valueSlicePool.Put(anySlice)
}
