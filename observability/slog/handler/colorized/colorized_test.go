// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// White-box tests: the deterministic units under test (buffer pool, LRU time
// cache, color cache, FNV hashing, lazy evaluators, custom level formatting)
// are unexported.
package colorized

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// fatih/color auto-detects TTY support at init, so without forcing a
	// baseline the plain-text output assertions would depend on the
	// environment the tests run in.
	color.NoColor = true
	os.Exit(m.Run())
}

type failWriter struct {
	err error
}

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

type stringerValue struct{}

func (stringerValue) String() string { return "stringy" }

// Handler tests.
//
// Tests that assert on formatted output do not call t.Parallel: the rendering
// depends on the process-global color.NoColor, which TestHandle_ColorizedOutput
// and TestWithNoColor_SetsGlobalFlag temporarily mutate.

func TestNewHandler_NilWriterPanics(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() { NewHandler(nil) })
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	// Default minimum level is slog.LevelError.
	h := NewHandler(io.Discard)
	require.True(t, h.Enabled(t.Context(), slog.LevelError))
	require.False(t, h.Enabled(t.Context(), slog.LevelWarn))

	h = NewHandler(io.Discard, WithLevel(slog.LevelInfo))
	require.True(t, h.Enabled(t.Context(), slog.LevelInfo))
	require.False(t, h.Enabled(t.Context(), slog.LevelDebug))
}

func TestHandle_ZeroTimeOmitted(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, WithLevel(slog.LevelDebug))

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "msg", 0)
	require.NoError(t, h.Handle(t.Context(), rec))

	require.Equal(t, "INF msg \n", buf.String())
}

func TestHandle_EndToEnd(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug)))

	logger.Info("hello world", "key", "value", "count", 42, "pi", 3.5, "ok", true)

	out := buf.String()
	require.True(t, strings.HasSuffix(out, "\n"))
	require.Contains(t, out, "INF")
	require.Contains(t, out, "hello world")
	require.Contains(t, out, "key=value")
	require.Contains(t, out, "count=42")
	require.Contains(t, out, "pi=3.5")
	require.Contains(t, out, "ok=true")
}

func TestHandle_Levels(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  string
	}{
		{level: slog.LevelDebug, want: "DBG"},
		{level: slog.LevelInfo, want: "INF"},
		{level: slog.LevelWarn, want: "WARN"},
		{level: slog.LevelError, want: "ERR"},
		{level: slog.Level(2), want: "INF+2"},
		{level: slog.Level(12), want: "ERR+4"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewHandler(&buf, WithLevel(slog.LevelDebug))

			rec := slog.NewRecord(time.Time{}, tc.level, "msg", 0)
			require.NoError(t, h.Handle(t.Context(), rec))
			require.Equal(t, tc.want+" msg \n", buf.String())
		})
	}
}

func TestHandle_AttributeQuoting(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug)))

	logger.Info("started", "spaced", "two words", "empty", "", "eq", "a=b")

	out := buf.String()
	require.Contains(t, out, `spaced="two words"`)
	require.Contains(t, out, `empty=""`)
	require.Contains(t, out, `eq="a=b"`)
	// The message itself is not quoted for spaces.
	require.Contains(t, out, "started")
	require.NotContains(t, out, `"started"`)
}

func TestHandle_ErrorAttribute(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug)))

	logger.Info("request failed", "error", errors.New("kaboom"))

	require.Contains(t, buf.String(), "error=kaboom")
}

func TestHandle_InlineGroupAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug)))

	logger.Info("m", slog.Group("g", slog.String("k", "v")))

	require.Contains(t, buf.String(), "g.k=v")
}

func TestWithGroup_Identity(t *testing.T) {
	t.Parallel()

	h := NewHandler(io.Discard)
	require.Same(t, h, h.WithGroup(""))
	require.Same(t, h, h.WithAttrs(nil))
}

// TestWithGroup_PrefixAppliesToSubsequentAttrs verifies the slog.Handler
// group contract: the group qualifies record attributes and attributes added
// after WithGroup, while attributes added before the group are never
// retroactively qualified.
func TestWithGroup_PrefixAppliesToSubsequentAttrs(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug)))

	// Record attrs are qualified starting from the very next record.
	base.WithGroup("req").Info("m", "id", 7)
	require.Contains(t, buf.String(), "req.id=7")

	// Attrs added after the group are qualified.
	buf.Reset()
	base.WithGroup("req").With("id", 7).Info("m")
	require.Contains(t, buf.String(), "req.id=7")

	// Attrs added before the group are never retroactively qualified.
	buf.Reset()
	base.With("app", "atlas").WithGroup("req").Info("m", "id", 7)
	out := buf.String()
	require.Contains(t, out, "app=atlas")
	require.NotContains(t, out, "req.app")
	require.Contains(t, out, "req.id=7")

	// Nested groups compose.
	buf.Reset()
	base.WithGroup("a").WithGroup("b").Info("m", "id", 7)
	require.Contains(t, buf.String(), "a.b.id=7")
}

func TestWithAttrs_ReplaceAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf,
		WithLevel(slog.LevelDebug),
		WithReplaceAttr(func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "password" {
				return slog.Attr{} // drop
			}
			if a.Key == "user" {
				a.Value = slog.StringValue("masked")
			}
			return a
		}),
	))

	logger.Info("login", "user", "bob", "password", "hunter2")

	out := buf.String()
	require.Contains(t, out, "user=masked")
	require.NotContains(t, out, "password")
	require.NotContains(t, out, "hunter2")
}

func TestHandle_PrefixAttribute(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf,
		WithLevel(slog.LevelDebug),
		WithPrefixAttributeKey("module"),
	))

	logger.Info("started", "module", "auth", "k", "v")

	out := buf.String()
	require.Contains(t, out, "auth")
	require.NotContains(t, out, "module=")
	require.Contains(t, out, "k=v")
	require.Less(t, strings.Index(out, "auth"), strings.Index(out, "started"),
		"prefix must be rendered before the message")
}

func TestHandle_AddSource(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug), WithAddSource()))

	logger.Info("m")

	out := buf.String()
	require.Contains(t, out, "source.file=")
	require.Contains(t, out, "source.line=")
	require.Contains(t, out, "colorized_test.go")
}

func TestHandle_WriteError(t *testing.T) {
	t.Parallel()

	sink := errors.New("sink failed")
	h := NewHandler(failWriter{err: sink})

	rec := slog.NewRecord(time.Time{}, slog.LevelError, "msg", 0)
	require.ErrorIs(t, h.Handle(t.Context(), rec), sink)
}

func TestHandle_Concurrent(t *testing.T) {
	const goroutines, perGoroutine = 8, 25

	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, WithLevel(slog.LevelDebug)))

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for i := range perGoroutine {
				logger.Info("concurrent", "i", i)
			}
		})
	}
	wg.Wait()

	require.Equal(t, goroutines*perGoroutine, strings.Count(buf.String(), "\n"))
}

// TestHandle_ColorizedOutput is serial: it mutates the process-global
// color.NoColor.
func TestHandle_ColorizedOutput(t *testing.T) {
	old := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = old })

	var buf bytes.Buffer
	h := NewHandler(&buf, WithLevel(slog.LevelDebug))

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "msg", 0)
	require.NoError(t, h.Handle(t.Context(), rec))
	require.Contains(t, buf.String(), "\x1b[", "expected ANSI escape sequences")
}

// TestWithNoColor_SetsGlobalFlag is serial: it mutates the process-global
// color.NoColor.
func TestWithNoColor_SetsGlobalFlag(t *testing.T) {
	old := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = old })

	NewHandler(io.Discard, WithNoColor())
	require.True(t, color.NoColor)
}

// Buffer tests.

func TestBuffer_PoolReuse(t *testing.T) {
	t.Parallel()

	b := newBuffer()
	b.writeString("data")
	require.Equal(t, "data", string(*b))
	b.Free()

	// Pooled buffers must always come back empty, whether or not the pool
	// hands out the same instance.
	b2 := newBuffer()
	require.Empty(t, *b2)
	b2.Free()
}

func TestBuffer_FreeSkipsOversized(t *testing.T) {
	t.Parallel()

	b := newBuffer()
	b.write(make([]byte, DefaultMaxBufferSize+1))
	b.Free()

	// Oversized buffers are not returned to the pool and keep their content.
	require.Len(t, *b, DefaultMaxBufferSize+1)
}

func TestBuffer_Finalize(t *testing.T) {
	t.Parallel()

	b := &buffer{}
	require.Empty(t, b.finalize(), "empty buffer must not get a newline")

	b.writeString("x")
	require.Equal(t, "x\n", string(b.finalize()))
}

func TestBuffer_NeedsQuoting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		isAttr bool
		want   bool
	}{
		{name: "empty attr", input: "", isAttr: true, want: true},
		{name: "empty message", input: "", isAttr: false, want: true},
		{name: "simple", input: "simple", isAttr: true, want: false},
		{name: "space in attr", input: "two words", isAttr: true, want: true},
		{name: "space in message", input: "two words", isAttr: false, want: false},
		{name: "equal sign", input: "a=b", isAttr: false, want: true},
		{name: "double quote", input: `say "hi"`, isAttr: false, want: true},
		{name: "newline", input: "line1\nline2", isAttr: false, want: true},
		{name: "invalid utf8", input: "\xff", isAttr: false, want: true},
		{name: "unicode letters", input: "привет", isAttr: true, want: false},
	}

	b := &buffer{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, b.needsQuoting(tc.input, tc.isAttr))
		})
	}
}

func TestBuffer_QuoteIfNeeded(t *testing.T) {
	t.Parallel()

	b := &buffer{}
	require.Equal(t, `"two words"`, b.quoteIfNeeded("two words", true))
	require.Equal(t, "plain", b.quoteIfNeeded("plain", true))
}

func TestBuffer_WriteValue(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value slog.Value
		want  string
	}{
		{name: "string", value: slog.StringValue("abc"), want: "abc"},
		{name: "string quoted", value: slog.StringValue("two words"), want: `"two words"`},
		{name: "int64", value: slog.Int64Value(-5), want: "-5"},
		{name: "uint64", value: slog.Uint64Value(7), want: "7"},
		{name: "float64", value: slog.Float64Value(1.5), want: "1.5"},
		{name: "bool", value: slog.BoolValue(true), want: "true"},
		{name: "duration", value: slog.DurationValue(1500 * time.Millisecond), want: "1.5s"},
		{name: "time", value: slog.TimeValue(fixed), want: fixed.String()},
		{name: "any stringer", value: slog.AnyValue(stringerValue{}), want: "stringy"},
		{name: "any int slice", value: slog.AnyValue([]int{1, 2}), want: `"[1 2]"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := &buffer{}
			b.writeValue(tc.value)
			require.Equal(t, tc.want, string(*b))
		})
	}
}

// Formatting utility tests.

func TestFormatCustomLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level slog.Level
		want  string
	}{
		{level: slog.LevelDebug, want: "DBG"},
		{level: slog.Level(-6), want: "DBG-2"},
		{level: slog.Level(-3), want: "DBG+1"},
		{level: slog.Level(1), want: "INF+1"},
		{level: slog.Level(5), want: "WARN+1"},
		{level: slog.LevelError, want: "ERR"},
		{level: slog.Level(9), want: "ERR+1"},
		{level: slog.Level(12), want: "ERR+4"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, formatCustomLevel(tc.level))
		})
	}
}

func TestAppendAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", value: nil, want: "<nil>"},
		{name: "bool", value: true, want: "true"},
		{name: "int", value: 42, want: "42"},
		{name: "int64", value: int64(-9), want: "-9"},
		{name: "uint", value: uint(5), want: "5"},
		{name: "float64", value: 2.5, want: "2.5"},
		{name: "string", value: "str", want: "str"},
		{name: "bytes", value: []byte("bs"), want: "bs"},
		{name: "error", value: errors.New("kaboom"), want: "kaboom"},
		{name: "int slice", value: []int{1, 2, 3}, want: "[1 2 3]"},
		{name: "empty int slice", value: []int{}, want: "[]"},
		{name: "string slice", value: []string{"a", "b"}, want: `["a" "b"]`},
		{name: "unknown type", value: struct{}{}, want: `"<unknown>"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, string(appendAny(nil, tc.value)))
		})
	}
}

// Hashing tests.

func TestHashString_KnownVectors(t *testing.T) {
	t.Parallel()

	// Published FNV-1a 32-bit test vectors.
	require.Equal(t, uint32(2166136261), hashString(""))
	require.Equal(t, uint32(0xe40c292c), hashString("a"))
	require.Equal(t, uint32(0xbf9cf968), hashString("foobar"))
}

func TestHashColorKey(t *testing.T) {
	t.Parallel()

	red := newColorKey([]color.Attribute{color.FgRed})
	require.Equal(t, hashColorKey(red), hashColorKey(red), "hash must be deterministic")

	blue := newColorKey([]color.Attribute{color.FgBlue})
	require.NotEqual(t, hashColorKey(red), hashColorKey(blue))

	require.Equal(t, uint32(fnv1aOffset), hashColorKey(newColorKey(nil)))
}

func TestNewColorKey_TruncatesAttributes(t *testing.T) {
	t.Parallel()

	attrs := make([]color.Attribute, 12)
	key := newColorKey(attrs)
	require.Equal(t, len(key.attrs), key.len)
}

// Time cache tests.

func TestShardedTimeCache_PutGet(t *testing.T) {
	t.Parallel()

	cache := newShardedTimeCache(4)
	key := timeCacheKey{unix: 123, format: "f"}

	_, ok := cache.get(key)
	require.False(t, ok)

	cache.put(key, "v1")
	got, ok := cache.get(key)
	require.True(t, ok)
	require.Equal(t, "v1", got)

	// Updating an existing key replaces the value without growing the LRU.
	cache.put(key, "v2")
	got, ok = cache.get(key)
	require.True(t, ok)
	require.Equal(t, "v2", got)
}

func TestShardedTimeCache_EvictsOldest(t *testing.T) {
	t.Parallel()

	cache := newShardedTimeCache(2)

	// Collect three keys that land in the same shard.
	target := cache.getShard(timeCacheKey{unix: 0, format: "f"})
	keys := make([]timeCacheKey, 0, 3)
	for u := int64(0); len(keys) < 3 && u < 10000; u++ {
		k := timeCacheKey{unix: u, format: "f"}
		if cache.getShard(k) == target {
			keys = append(keys, k)
		}
	}
	require.Len(t, keys, 3)

	cache.put(keys[0], "v0")
	cache.put(keys[1], "v1")
	cache.put(keys[2], "v2")

	_, ok := cache.get(keys[0])
	require.False(t, ok, "oldest entry must be evicted")
	for _, k := range keys[1:] {
		_, ok := cache.get(k)
		require.True(t, ok)
	}
}

// Color cache tests.

func TestColorCache_PutGet(t *testing.T) {
	t.Parallel()

	c := newColorCache()
	key := newColorKey([]color.Attribute{color.FgRed})

	require.Nil(t, c.getColor(key))

	col := color.New(color.FgRed)
	c.putColor(key, unsafe.Pointer(col))
	require.Equal(t, unsafe.Pointer(col), c.getColor(key))
}

func TestColorCache_SlotCollisionUsesOverflow(t *testing.T) {
	t.Parallel()

	c := newColorCache()

	base := newColorKey([]color.Attribute{color.Attribute(1)})
	slot := hashColorKey(base) & 255

	var other colorKey
	found := false
	for a := 2; a < 100000; a++ {
		k := newColorKey([]color.Attribute{color.Attribute(a)})
		if hashColorKey(k)&255 == slot {
			other, found = k, true
			break
		}
	}
	require.True(t, found, "expected a slot collision within the search space")

	first := color.New(color.Attribute(1))
	second := color.New(color.FgHiWhite)
	c.putColor(base, unsafe.Pointer(first))
	c.putColor(other, unsafe.Pointer(second))

	require.Equal(t, unsafe.Pointer(first), c.getColor(base))
	require.Equal(t, unsafe.Pointer(second), c.getColor(other), "collided key must be served from overflow")
	require.Equal(t, unsafe.Pointer(second), c.getColor(other), "repeat lookups stay consistent")
}

func TestGetColor_CachesInstances(t *testing.T) {
	t.Parallel()

	require.Nil(t, getColor())

	c1 := getColor(color.FgMagenta, color.Bold)
	c2 := getColor(color.FgMagenta, color.Bold)
	require.NotNil(t, c1)
	require.Same(t, c1, c2)
}

// Time formatting tests.

func TestFormatTime(t *testing.T) {
	t.Parallel()

	require.Empty(t, formatTime(time.Time{}, time.RFC3339))

	fixed := time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC)
	want := fixed.Format(time.RFC3339)
	require.Equal(t, want, formatTime(fixed, time.RFC3339))
	// Second call is served from the cache.
	require.Equal(t, want, formatTime(fixed, time.RFC3339))

	require.Equal(t, "2026-07-12", formatTime(fixed, "2006-01-02"))
}

// TestFormatTime_SubSecondPrecision verifies that sub-second formats are
// cached at nanosecond granularity: two distinct instants within the same
// millisecond must format independently.
func TestFormatTime_SubSecondPrecision(t *testing.T) {
	t.Parallel()

	base := time.Date(2031, 3, 5, 7, 9, 11, 500_000_000, time.UTC)
	later := base.Add(123456 * time.Nanosecond)
	require.Equal(t, base.UnixMilli(), later.UnixMilli())

	require.Equal(t, base.Format(time.RFC3339Nano), formatTime(base, time.RFC3339Nano))
	require.Equal(t, later.Format(time.RFC3339Nano), formatTime(later, time.RFC3339Nano))
}

// TestFormatTime_LocationAware verifies that equal instants rendered in
// different time zones do not collide in the cache.
func TestFormatTime_LocationAware(t *testing.T) {
	t.Parallel()

	utc := time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC)
	zoned := utc.In(time.FixedZone("UTC+3", 3*3600))
	require.Equal(t, utc.Unix(), zoned.Unix())

	require.Equal(t, utc.Format(time.RFC3339), formatTime(utc, time.RFC3339))
	require.Equal(t, zoned.Format(time.RFC3339), formatTime(zoned, time.RFC3339))
}

func TestFormatHasSubSecond(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{name: "rfc3339nano", format: time.RFC3339Nano, want: true},
		{name: "stamp milli", format: time.StampMilli, want: true},
		{name: "custom micros", format: "15:04:05.000000", want: true},
		{name: "comma fraction", format: "15:04:05,999", want: true},
		{name: "rfc3339", format: time.RFC3339, want: false},
		// Dots followed by further digits are date separators, not fractions.
		{name: "dotted date", format: "2006.01.02", want: false},
		{name: "empty", format: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, formatHasSubSecond(tc.format))
		})
	}
}

// Lazy evaluator tests.

func TestBuildGroupPrefix(t *testing.T) {
	t.Parallel()

	require.Empty(t, buildGroupPrefix(nil))
	require.Empty(t, buildGroupPrefix([]string{}))
	require.Equal(t, "a.", buildGroupPrefix([]string{"a"}))
	require.Equal(t, "a.b.", buildGroupPrefix([]string{"a", "b"}))
}

func TestLazyGroupPrefix(t *testing.T) {
	t.Parallel()

	require.Empty(t, newLazyGroupPrefix(nil).get())

	p := newLazyGroupPrefix([]string{"x", "y"})
	require.Equal(t, "x.y.", p.get())
	require.Equal(t, "x.y.", p.get())
}

func TestLazyColorMap(t *testing.T) {
	t.Parallel()

	require.Nil(t, newLazyColorMap(nil).get())

	m := newLazyColorMap(map[string][]int{"k": {31, 1}})
	want := map[string][]color.Attribute{"k": {color.Attribute(31), color.Attribute(1)}}
	require.Equal(t, want, m.get())
	require.Equal(t, want, m.get())
}

func TestLazySource(t *testing.T) {
	t.Parallel()

	pc, _, _, ok := runtime.Caller(0)
	require.True(t, ok)

	s := newLazySource(pc)
	attr := s.get()
	require.Equal(t, slog.SourceKey, attr.Key)

	values := make(map[string]slog.Value, 3)
	for _, a := range attr.Value.Group() {
		values[a.Key] = a.Value
	}
	require.Contains(t, values["file"].String(), "colorized_test.go")
	require.Positive(t, values["line"].Int64())
	require.Contains(t, values["func"].String(), "TestLazySource")

	require.True(t, attr.Equal(s.get()), "repeated calls return the same attribute")
}
