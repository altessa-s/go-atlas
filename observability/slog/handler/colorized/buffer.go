// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package colorized

import (
	"encoding"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/fatih/color"
)

// Buffer pool configuration constants.
const (
	// DefaultBufferSize is the default initial buffer size (1KB).
	DefaultBufferSize = 1024
	// DefaultMaxBufferSize is the maximum buffer size kept in pool (16KB).
	DefaultMaxBufferSize = 16 << 10
)

// buffer represents a reusable byte buffer for log formatting
type buffer []byte

var bufferPool = &sync.Pool{
	New: func() any {
		b := make(buffer, 0, DefaultBufferSize)
		return &b
	},
}

// Pre-allocate common strings to avoid repeated allocations
var (
	equalSign  = []byte("=")
	spaceBytes = []byte(" ")
	newLine    = []byte("\n")

	// Pre-computed level strings
	levelStrings = map[slog.Level]string{
		slog.LevelDebug: "DBG",
		slog.LevelInfo:  "INF",
		slog.LevelWarn:  "WARN",
		slog.LevelError: "ERR",
	}

	// Pre-computed level colors
	levelColors = map[slog.Level]color.Attribute{
		slog.LevelDebug: color.BgBlue,
		slog.LevelInfo:  color.BgGreen,
		slog.LevelWarn:  color.BgYellow,
		slog.LevelError: color.BgRed,
	}
)

// newBuffer gets a buffer from the pool
func newBuffer() *buffer {
	return bufferPool.Get().(*buffer) //nolint:errcheck
}

// Free returns the buffer to the pool if it's not too large
func (b *buffer) Free() {
	if cap(*b) <= DefaultMaxBufferSize {
		*b = (*b)[:0]
		bufferPool.Put(b)
	}
}

// Low-level write methods optimized for performance

func (b *buffer) write(p []byte) {
	*b = append(*b, p...)
}

func (b *buffer) writeString(s string) {
	*b = append(*b, s...)
}

// writeBytes is an optimization for common byte sequences
func (b *buffer) writeBytes(bytes []byte) {
	*b = append(*b, bytes...)
}

// Write implements io.Writer for compatibility
func (b *buffer) Write(p []byte) (int, error) {
	b.write(p)
	return len(p), nil
}

// writeColorized writes a string with color formatting
func (b *buffer) writeColorized(s string, attrs ...color.Attribute) {
	if len(attrs) == 0 || color.NoColor {
		b.writeString(s)
		return
	}

	if c := getColor(attrs...); c != nil {
		_, _ = c.Fprint(b, s)
	} else {
		b.writeString(s)
	}
}

// Specialized write methods for log components

// writeTime formats and writes a time value using the time cache for performance
func (b *buffer) writeTime(t time.Time, format string) {
	if format == "" {
		format = time.RFC3339Nano
	}
	// Use cached time formatting
	b.writeColorized(formatTime(t, format), color.FgWhite)
}

// writeLevel writes a log level with appropriate coloring
func (b *buffer) writeLevel(level slog.Level) {
	// Get pre-computed string or format custom level
	levelStr, ok := levelStrings[level]
	if !ok {
		// Handle custom levels without allocation
		levelStr = formatCustomLevel(level)
	}

	// Get color from map or compute for custom levels
	bgColor := levelColors[level]
	if bgColor == 0 {
		switch {
		case level < slog.LevelInfo:
			bgColor = color.BgBlue
		case level < slog.LevelWarn:
			bgColor = color.BgGreen
		case level < slog.LevelError:
			bgColor = color.BgYellow
		default:
			bgColor = color.BgRed
		}
	}

	b.writeColorized(levelStr, bgColor)
}

// writeMessage writes a log message with proper quoting if needed
func (b *buffer) writeMessage(msg string) {
	b.writeColorized(b.quoteIfNeeded(msg, false), color.FgBlack)
}

// writeKey writes an attribute key with coloring
func (b *buffer) writeKey(key string, colorAttrs []color.Attribute) {
	if len(colorAttrs) == 0 {
		colorAttrs = []color.Attribute{color.FgCyan}
	}
	b.writeColorized(key, colorAttrs...)
	b.writeBytes(equalSign)
}

// writeValue writes an attribute value with proper formatting
func (b *buffer) writeValue(v slog.Value) {
	switch v.Kind() {
	case slog.KindString:
		b.writeString(b.quoteIfNeeded(v.String(), true))
	case slog.KindInt64:
		*b = strconv.AppendInt(*b, v.Int64(), 10)
	case slog.KindUint64:
		*b = strconv.AppendUint(*b, v.Uint64(), 10)
	case slog.KindFloat64:
		*b = strconv.AppendFloat(*b, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		*b = strconv.AppendBool(*b, v.Bool())
	case slog.KindDuration:
		b.writeString(v.Duration().String())
	case slog.KindTime:
		b.writeString(v.Time().String())
	case slog.KindAny:
		b.writeString(b.quoteIfNeeded(b.anyToString(v.Any()), true))
	default:
		// Use zero-allocation append for unknown types
		start := len(*b)
		*b = appendAny(*b, v.Any())
		if b.needsQuoting(string((*b)[start:]), true) {
			quoted := strconv.Quote(string((*b)[start:]))
			*b = (*b)[:start]
			b.writeString(quoted)
		}
	}
}

// writeErrorValue writes an error value with special formatting
func (b *buffer) writeErrorValue(v any) {
	var errStr string
	switch e := v.(type) {
	case error:
		errStr = e.Error()
	case string:
		errStr = e
	default:
		// Use zero-allocation conversion
		buf := make([]byte, 0, 64)
		buf = appendAny(buf, e)
		errStr = string(buf)
	}
	b.writeColorized(b.quoteIfNeeded(errStr, true), color.FgRed, color.Bold)
}

// writeAttribute writes a complete key=value attribute
func (b *buffer) writeAttribute(key string, val slog.Value, colorMap map[string][]color.Attribute) {
	// Special handling for error attribute
	if key == "error" {
		b.writeKey(key, colorMap[key])
		b.writeErrorValue(val.Any())
		return
	}

	b.writeKey(key, colorMap[key])
	b.writeValue(val)
}

// Utility methods

// quoteIfNeeded quotes a string if it contains special characters
func (b *buffer) quoteIfNeeded(s string, isAttr bool) string {
	if !b.needsQuoting(s, isAttr) {
		return s
	}

	// Use strconv.Quote for safety and correctness
	return strconv.Quote(s)
}

// needsQuoting checks if a string needs to be quoted
func (b *buffer) needsQuoting(s string, isAttr bool) bool {
	if len(s) == 0 {
		return true
	}

	// Fast path: check for common cases
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError || r == '"' || r == '=' {
			return true
		}
		if isAttr && unicode.IsSpace(r) {
			return true
		}
		if !unicode.IsPrint(r) && !unicode.IsGraphic(r) {
			return true
		}
		i += size
	}

	return false
}

// anyToString converts any value to a string representation
func (b *buffer) anyToString(v any) string {
	// Check for common interfaces first
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case encoding.TextMarshaler:
		if data, err := x.MarshalText(); err == nil {
			return string(data)
		}
	}

	// Use zero-allocation conversion for common types
	buf := make([]byte, 0, 64)
	buf = appendAny(buf, v)
	return string(buf)
}

// High-level formatting methods

// formatBuiltinAttr formats time, level, and message attributes
func (b *buffer) formatBuiltinAttr(a slog.Attr, timeFormat string) {
	switch a.Key {
	case slog.TimeKey:
		if a.Value.Kind() == slog.KindTime {
			b.writeTime(a.Value.Time(), timeFormat)
		} else {
			// Handle other time representations
			b.writeColorized(b.anyToString(a.Value.Any()), color.FgWhite)
		}
	case slog.LevelKey:
		if a.Value.Kind() == slog.KindInt64 {
			b.writeLevel(slog.Level(a.Value.Int64()))
		} else if leveler, ok := a.Value.Any().(slog.Leveler); ok {
			b.writeLevel(leveler.Level())
		} else {
			b.writeString(b.quoteIfNeeded(b.anyToString(a.Value.Any()), true))
		}
	case slog.MessageKey:
		b.writeMessage(a.Value.String())
	}
	b.writeBytes(spaceBytes)
}

// formatAttributes formats a list of attributes
func (b *buffer) formatAttributes(attrs []slog.Attr, groupPrefix string, colorMap map[string][]color.Attribute) {
	for i, attr := range attrs {
		if i > 0 {
			b.writeBytes(spaceBytes)
		}
		b.formatAttribute(attr, groupPrefix, colorMap)
	}
}

// formatAttribute formats a single attribute, handling groups recursively
func (b *buffer) formatAttribute(attr slog.Attr, groupPrefix string, colorMap map[string][]color.Attribute) {
	// Skip empty attributes
	if attr.Equal(slog.Attr{}) {
		return
	}

	// Lazy resolution of value only when needed
	val := attr.Value.Resolve()
	key := groupPrefix + attr.Key

	switch val.Kind() {
	case slog.KindGroup:
		// Recursively format group attributes
		b.formatAttributes(val.Group(), key+".", colorMap)
	default:
		b.writeAttribute(key, val, colorMap)
	}
}

// finalize adds a newline and returns the buffer contents
func (b *buffer) finalize() []byte {
	if len(*b) > 0 {
		b.writeBytes(newLine)
	}
	return *b
}
