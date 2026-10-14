// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"bytes"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type (
	definedString string
	definedBytes  []byte
	structPayload struct {
		User string
		Pass []byte
	}
)

// TestValueClone checks that clearing a clone leaves the original intact for
// every payload kind Clear handles, and the other way round.
func TestValueClone(t *testing.T) {
	t.Parallel()

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		orig := NewValue("key", string([]byte("payload")), []byte("encoded"), "v1")
		orig.EncodedKey = "enc-key"
		c := orig.clone()
		require.Equal(t, orig.Key, c.Key)
		require.Equal(t, "enc-key", c.EncodedKey)
		require.Equal(t, orig.Version, c.Version)
		require.Equal(t, "payload", c.Value)

		c.Clear()
		require.Equal(t, "payload", orig.Value)
		require.Equal(t, "encoded", string(orig.EncodedValue))

		c2 := orig.clone()
		orig.Clear()
		require.Equal(t, "payload", c2.Value)
		require.Equal(t, "encoded", string(c2.EncodedValue))
	})

	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		orig := NewValue("key", []byte("payload"), nil, "v1")
		c := orig.clone()
		require.Nil(t, c.EncodedValue)
		c.Clear()
		require.Equal(t, "payload", string(orig.Value))
	})

	t.Run("defined string", func(t *testing.T) {
		t.Parallel()
		orig := NewValue("key", definedString([]byte("payload")), nil, "v1")
		c := orig.clone()
		c.Clear()
		require.Equal(t, definedString("payload"), orig.Value)
	})

	t.Run("defined bytes", func(t *testing.T) {
		t.Parallel()
		orig := NewValue("key", definedBytes("payload"), nil, "v1")
		c := orig.clone()
		c.Clear()
		require.Equal(t, definedBytes("payload"), orig.Value)
	})

	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		orig := NewValue("key", structPayload{User: "u", Pass: []byte("p")}, nil, "v1")
		c := orig.clone()
		c.Clear()
		require.Equal(t, structPayload{User: "u", Pass: []byte("p")}, orig.Value)
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()
		p := &structPayload{User: "u"}
		orig := NewValue("key", p, nil, "v1")
		c := orig.clone()
		c.Clear()
		require.Same(t, p, orig.Value)
		require.Equal(t, "u", p.User)
	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		var v *Value[string]
		require.Nil(t, v.clone())
	})
}

// TestNewValue_DefinedStringType checks that NewValue accepts a payload of a
// defined string type.
func TestNewValue_DefinedStringType(t *testing.T) {
	t.Parallel()

	v := NewValue("key", definedString("payload"), nil, "v1")
	require.Equal(t, definedString("payload"), v.Value)
	v.Clear()
	require.Empty(t, v.Value)
}

// TestValueClear_ZeroesSecretMemory checks that Clear zeroes the encoded bytes
// and the string payload in place, and that a clone owns separate memory.
func TestValueClear_ZeroesSecretMemory(t *testing.T) {
	t.Parallel()

	payload := []byte("payload")
	encoded := []byte("encoded")
	orig := NewValue("key", string(payload), encoded, "v1")
	c := orig.clone()
	cEncoded := c.EncodedValue
	cPayload := unsafe.Slice(unsafe.StringData(c.Value), len(c.Value))

	orig.Clear()
	require.Equal(t, make([]byte, len(encoded)), encoded)
	require.Empty(t, orig.Value)
	require.Nil(t, orig.EncodedValue)
	require.Equal(t, "encoded", string(cEncoded))
	require.Equal(t, "payload", string(cPayload))

	c.Clear()
	require.Equal(t, make([]byte, len(cEncoded)), cEncoded)
	require.Equal(t, make([]byte, len(cPayload)), cPayload)

	// A second Clear is a no-op.
	c.Clear()
}

// TestNewValue_CleanupZeroesEncodedValue checks that an unreachable Value
// that was never cleared still has its encoded bytes zeroed by the GC cleanup.
func TestNewValue_CleanupZeroesEncodedValue(t *testing.T) {
	t.Parallel()
	if raceEnabled {
		// The cleanup goroutine's write has no happens-before edge the race
		// detector can see, so polling the bytes is reported as a race.
		t.Skip("GC cleanup observation is not race-detector clean")
	}

	encoded := []byte("encoded-secret")
	func() {
		v := NewValue("key", string([]byte("payload")), encoded, "v1")
		runtime.KeepAlive(v)
	}()

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		runtime.GC()
		return bytes.Equal(encoded, make([]byte, len(encoded)))
	}, "encoded value not zeroed by cleanup")
}
