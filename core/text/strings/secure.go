// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"crypto/subtle"
	"runtime/debug"
	"sync"
	"unsafe"

	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
)

const (
	// DefaultSmallStringOptimizationThreshold is the maximum byte length of a
	// string that [SecureString] stores in its fixed-size inline buffer instead
	// of a heap-allocated slice. Strings at or below this threshold benefit from
	// better cache locality and avoid a separate heap allocation.
	DefaultSmallStringOptimizationThreshold = 64

	// OptimizedMemoryZeroingThreshold is the byte-length cutoff below which a
	// manual zeroing loop is faster than the built-in clear() function. Used
	// internally by [SecureString.Clear] and [ZeroBytes].
	OptimizedMemoryZeroingThreshold = 32

	// EmptyStringValue is the canonical empty string returned by [SecureString]
	// methods when no data is stored.
	EmptyStringValue = ""

	// SecureStringGoStringEmpty is the value returned by
	// [SecureString.GoString] when the instance contains no data.
	SecureStringGoStringEmpty = "SecureString{<empty>}"

	// SecureStringGoStringRedacted is the value returned by
	// [SecureString.GoString] for non-empty instances, preventing accidental
	// exposure of sensitive data in debug output or logs.
	SecureStringGoStringRedacted = "SecureString{<redacted>}"

	// ZeroByte is the byte value written during memory clearing operations.
	ZeroByte = byte(0)

	// ConstantTimeEqual is the return value from [crypto/subtle.ConstantTimeCompare]
	// that indicates two byte slices are equal.
	ConstantTimeEqual = 1
)

var secureStringPool = sync.Pool{
	New: func() any {
		return &SecureString{}
	},
}

// SecureString provides tamper-resistant, zeroing-on-clear storage for
// sensitive string data such as passwords, tokens, and cryptographic keys.
//
// Key properties:
//   - Explicit memory zeroing via [SecureString.Clear] prevents sensitive data
//     from lingering in process memory after use.
//   - Small-string optimization: strings up to
//     [DefaultSmallStringOptimizationThreshold] bytes are stored inline,
//     avoiding a separate heap allocation and improving cache locality.
//   - The [fmt.GoStringer] interface is implemented to print a redacted
//     placeholder, preventing accidental exposure in logs or debug output.
//   - Object pooling: instances created with [NewSecureString] are drawn from
//     and returned to an internal [sync.Pool] when [SecureString.Clear] is
//     called, reducing GC pressure.
//   - A runtime cleanup is registered to zero heap-allocated data if the
//     instance becomes unreachable without an explicit [SecureString.Clear].
//
// Callers must invoke [SecureString.Clear] (typically via defer) when the
// sensitive data is no longer needed. After Clear the instance must not be
// reused.
//
// SecureString is safe for concurrent reads but must not be written to
// concurrently; create separate instances for each goroutine or protect
// with external synchronization.
//
// Example:
//
//	ss := NewSecureString("password")
//	defer ss.Clear()
//	value := ss.String()
type SecureString struct {
	// For small strings, store inline to avoid heap allocation and improve cache locality
	inline             [DefaultSmallStringOptimizationThreshold]byte
	data               []byte // Used for strings larger than SmallStringThreshold
	length             int    // Actual length of the stored data
	useInlineStorage   bool   // True if using inline storage, false if using heap storage
	originatesFromPool bool   // True if instance came from pool, affects cleanup behavior
	cleanup            coreruntime.Cleanup
}

// initializeSecureStringStorage initializes a SecureString with provided data.
func initializeSecureStringStorage(ss *SecureString, s string, fromPool bool) {
	ss.length = len(s)
	ss.originatesFromPool = fromPool

	// Use small string optimization for better performance
	if len(s) <= DefaultSmallStringOptimizationThreshold {
		ss.useInlineStorage = true
		if fromPool {
			ss.data = nil // Ensure heap storage is cleared for pooled instances
		}
		copy(ss.inline[:], s)
	} else {
		ss.useInlineStorage = false
		// For pooled instances, try to reuse existing slice if capacity is sufficient
		if fromPool && cap(ss.data) >= len(s) {
			ss.data = ss.data[:len(s)]
		} else {
			ss.data = make([]byte, len(s))
		}
		copy(ss.data, s)
	}
}

// NewSecureString creates a [SecureString] initialized with the given
// sensitive data. The instance is drawn from an internal [sync.Pool] to
// reduce allocation pressure. A runtime cleanup is registered so that, if
// the instance is garbage collected without an explicit [SecureString.Clear],
// the underlying bytes are still zeroed.
//
// Callers must call [SecureString.Clear] when the data is no longer needed.
// For an empty string, no cleanup is registered and the instance is returned
// immediately.
//
// Example:
//
//	ss := NewSecureString("password")
//	defer ss.Clear()
func NewSecureString(s string) *SecureString {
	ss := secureStringPool.Get().(*SecureString) //nolint:errcheck

	if s == EmptyStringValue {
		ss.length = 0
		ss.useInlineStorage = true
		ss.originatesFromPool = true
		return ss
	}

	initializeSecureStringStorage(ss, s, true)

	// Register cleanup for larger strings or safety
	ss.cleanup = coreruntime.AddCleanup(ss, cleanupSecureString, ss.data)
	return ss
}

func cleanupSecureString(data []byte) {
	if data != nil {
		ZeroBytes(data)
	}
}

// String returns a copy of the stored sensitive data as a Go string. A new
// allocation is made each call so the returned string is independent of the
// [SecureString] and survives [SecureString.Clear]. For hot paths where an
// allocation is undesirable, see [SecureString.StringUnsafe].
//
// The returned string should not be stored in long-lived data structures or
// logged, as it contains plaintext sensitive material.
//
// Example:
//
//	value := ss.String()
func (ss *SecureString) String() string {
	if ss.length == 0 {
		return EmptyStringValue
	}

	if ss.useInlineStorage {
		return string(ss.inline[:ss.length])
	}
	return string(ss.data)
}

// StringUnsafe returns the stored data as a string without copying the
// underlying bytes. The returned string shares memory with the [SecureString];
// it becomes invalid (contents zeroed) after [SecureString.Clear] is called.
//
// WARNING: The caller must not retain the returned string beyond the lifetime
// of the SecureString. This method exists for performance-critical paths where
// the zero-copy guarantee is worth the lifetime constraint.
//
// Example:
//
//	value := ss.StringUnsafe()  // no allocation
//
// #nosec G103 -- intentional zero-copy for performance
func (ss *SecureString) StringUnsafe() string {
	if ss.length == 0 {
		return EmptyStringValue
	}

	if ss.useInlineStorage {
		return unsafe.String(&ss.inline[0], ss.length)
	}
	return unsafe.String(&ss.data[0], len(ss.data))
}

// Bytes returns a newly allocated copy of the stored sensitive data as a byte
// slice. The copy is independent of the [SecureString] and survives
// [SecureString.Clear]. The caller is responsible for zeroing the returned
// slice (e.g. with [ZeroBytes]) when it is no longer needed.
//
// Returns nil when the SecureString is empty.
//
// Example:
//
//	data := ss.Bytes()
//	defer ZeroBytes(data)
func (ss *SecureString) Bytes() []byte {
	if ss.length == 0 {
		return nil
	}

	result := make([]byte, ss.length)
	if ss.useInlineStorage {
		copy(result, ss.inline[:ss.length])
	} else {
		copy(result, ss.data)
	}
	return result
}

// BytesUnsafe returns the internal byte storage without copying. Modifying the
// returned slice directly mutates the [SecureString]. The slice becomes invalid
// (zeroed) after [SecureString.Clear] is called.
//
// Returns nil when the SecureString is empty.
//
// WARNING: The caller must not retain or modify the slice beyond the lifetime
// of the SecureString.
//
// Example:
//
//	data := ss.BytesUnsafe()  // no allocation
func (ss *SecureString) BytesUnsafe() []byte {
	if ss.length == 0 {
		return nil
	}

	if ss.useInlineStorage {
		return ss.inline[:ss.length]
	}
	return ss.data
}

// IsEmpty reports whether the [SecureString] contains zero bytes of data.
// Returns true after [SecureString.Clear] has been called.
func (ss *SecureString) IsEmpty() bool {
	return ss.length == 0
}

// Len returns the length of the stored sensitive data in bytes. Returns 0
// after [SecureString.Clear] has been called.
func (ss *SecureString) Len() int {
	return ss.length
}

// Equal performs a constant-time comparison of this [SecureString] with other
// using [crypto/subtle.ConstantTimeCompare]. Returns true only when both
// instances hold byte-identical data. Two empty SecureStrings are considered
// equal. If the lengths differ, the comparison short-circuits to false.
//
// Use this method instead of comparing the results of [SecureString.String]
// to avoid timing side-channels when verifying passwords or tokens.
//
// Example:
//
//	if stored.Equal(provided) { ... }
func (ss *SecureString) Equal(other *SecureString) bool {
	if ss.length == 0 && other.length == 0 {
		return true
	}
	if ss.length != other.length {
		return false
	}

	// Get byte slices for constant-time comparison
	var currentBytes, otherBytes []byte
	if ss.useInlineStorage {
		currentBytes = ss.inline[:ss.length]
	} else {
		currentBytes = ss.data
	}

	if other.useInlineStorage {
		otherBytes = other.inline[:other.length]
	} else {
		otherBytes = other.data
	}

	return subtle.ConstantTimeCompare(currentBytes, otherBytes) == ConstantTimeEqual
}

// Clear overwrites the stored sensitive data with zeros, cancels any runtime
// cleanup, and returns the instance to the internal pool if it was created by
// [NewSecureString]. After Clear returns, the [SecureString] must not be used
// -- all accessor methods will return zero-length results.
//
// Calling Clear on an already-cleared instance is safe and has no effect.
//
// Example:
//
//	ss := NewSecureString("password")
//	defer ss.Clear()
func (ss *SecureString) Clear() {
	// Store pool status before clearing (needed for pool return decision)
	originatedFromPool := ss.originatesFromPool

	if ss.length > 0 {
		if ss.useInlineStorage {
			// Clear inline storage with optimized strategy
			if ss.length <= OptimizedMemoryZeroingThreshold {
				// Manual loop is faster for small data
				for i := range ss.length {
					ss.inline[i] = ZeroByte
				}
			} else {
				// Use clear() builtin for larger data
				clear(ss.inline[:ss.length])
			}
		} else if ss.data != nil {
			// Clear heap storage with optimized strategy
			if len(ss.data) <= OptimizedMemoryZeroingThreshold {
				// Manual loop is faster for small data
				for i := range ss.data {
					ss.data[i] = ZeroByte
				}
			} else {
				// Use clear() builtin for larger data (2-3x faster)
				clear(ss.data)
			}
			ss.data = nil
		}
		ss.length = 0
	}

	// Reset state
	ss.originatesFromPool = false
	if ss.cleanup != nil {
		ss.cleanup.Stop()
		ss.cleanup = nil
	}

	// Automatically return to pool if instance originally came from pool
	// This provides seamless pool management without separate ReturnToPool() method
	if originatedFromPool {
		secureStringPool.Put(ss)
	}
}

// GoString implements [fmt.GoStringer] to prevent accidental exposure of
// sensitive data in debug output, fmt.Printf("%#v"), or structured logging.
// Returns [SecureStringGoStringEmpty] for empty instances and
// [SecureStringGoStringRedacted] otherwise.
func (ss *SecureString) GoString() string {
	if ss.IsEmpty() {
		return SecureStringGoStringEmpty
	}
	return SecureStringGoStringRedacted
}

// ZeroBytes overwrites every element of data with zero to prevent sensitive
// material from remaining in process memory. For slices at or below
// [OptimizedMemoryZeroingThreshold] bytes a manual loop is used; larger slices
// use the built-in clear() for better throughput. Passing a nil or empty slice
// is a no-op.
//
// Example:
//
//	data := ss.Bytes()
//	defer ZeroBytes(data)
func ZeroBytes(data []byte) {
	if len(data) == 0 {
		return
	}

	// For small slices, manual loop is the fastest due to reduced overhead
	if len(data) <= OptimizedMemoryZeroingThreshold {
		for i := range data {
			data[i] = ZeroByte
		}
		return
	}

	// For larger slices, use clear() builtin (Go 1.21+) - significantly faster
	clear(data)
}

// ZeroString attempts to overwrite the bytes backing s with zeros using unsafe
// pointer arithmetic. This is a best-effort operation: if s resides in
// read-only memory (e.g. a string literal or a constant folded by the
// compiler), the write will cause a panic that is silently recovered.
//
// For reliable zeroing, prefer [SecureString] which always owns its backing
// storage. ZeroString is most useful as a defense-in-depth measure for
// heap-allocated strings obtained from I/O (user input, network reads, etc.).
//
// Passing an empty string is a no-op.
//
// Example:
//
//	password := getUserInput()
//	defer ZeroString(password)
//
// #nosec G103 -- intentional unsafe for secure memory zeroing
func ZeroString(s string) {
	if s == EmptyStringValue {
		return
	}

	// Use unsafe operations to avoid the allocation that []byte(s) would cause.
	// This directly accesses the string's underlying byte array.
	//
	// SAFETY: Writing to read-only memory (e.g. string literals in the rodata
	// segment) causes a SIGSEGV. By default Go treats a SIGSEGV on a non-nil
	// address as fatal (runtime.throw) which recover() cannot catch.
	// debug.SetPanicOnFault converts such faults into recoverable panics.
	prev := debug.SetPanicOnFault(true)
	defer debug.SetPanicOnFault(prev)

	defer func() {
		if r := recover(); r != nil {
			// String was likely a literal or in read-only memory — silently ignore.
			_ = r
		}
	}()

	// Convert string to byte slice without allocation using unsafe
	data := unsafe.Slice(unsafe.StringData(s), len(s))

	// Clear the memory using our optimized ZeroBytes function
	ZeroBytes(data)
}
