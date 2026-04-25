// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

// defaultPoolCapacity is the default capacity for pooled string slices.
const defaultPoolCapacity = 64

var defaultStringSlicePool = slices.NewPool[string](defaultPoolCapacity)

// GetStringSlice retrieves a pre-allocated string slice from the package-level
// pool and resets its length to zero while retaining its capacity. The caller
// must return the slice with [PutStringSlice] when finished to enable reuse.
// The returned slice has an initial capacity of at least defaultPoolCapacity.
func GetStringSlice() *[]string {
	return defaultStringSlicePool.Get()
}

// GetStringSliceWithCapacity retrieves a string slice from the pool, ensuring
// the returned slice has at least expectedCapacity elements of capacity. If the
// pooled slice is too small, a new one is allocated. The caller must return the
// slice with [PutStringSlice] when finished.
func GetStringSliceWithCapacity(expectedCapacity int) *[]string {
	return defaultStringSlicePool.GetWithCapacity(expectedCapacity)
}

// PutStringSlice returns a string slice to the package-level pool for reuse.
// The slice's element references are cleared to allow garbage collection of
// the referenced strings. After calling PutStringSlice the caller must not
// use the slice again.
func PutStringSlice(slice *[]string) {
	defaultStringSlicePool.Put(slice)
}

// StringBuilderPool is a package-level [sync.Pool] that recycles
// [strings.Builder] instances to reduce allocation pressure. Prefer using
// [GetStringBuilder] and [PutStringBuilder] for correct reset semantics.
var StringBuilderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// GetStringBuilder retrieves a [strings.Builder] from [StringBuilderPool],
// resets it, and returns it ready for use. The caller must return it with
// [PutStringBuilder] when finished. If the pool is empty, a new Builder is
// allocated.
func GetStringBuilder() *strings.Builder {
	builder, ok := StringBuilderPool.Get().(*strings.Builder)
	if !ok {
		builder = &strings.Builder{}
	}
	builder.Reset()
	return builder
}

// PutStringBuilder resets builder and returns it to [StringBuilderPool] for
// reuse. Passing nil is a no-op. After calling PutStringBuilder the caller
// must not use the builder again.
func PutStringBuilder(builder *strings.Builder) {
	if builder == nil {
		return
	}

	builder.Reset()
	StringBuilderPool.Put(builder)
}

// BuildString borrows a [strings.Builder] from [StringBuilderPool], passes it
// to buildFunc, and returns the resulting string. The builder is automatically
// returned to the pool when buildFunc completes. This is a convenience wrapper
// that eliminates manual [GetStringBuilder]/[PutStringBuilder] pairing.
func BuildString(buildFunc func(*strings.Builder)) string {
	builder := GetStringBuilder()
	defer PutStringBuilder(builder)

	buildFunc(builder)
	return builder.String()
}
