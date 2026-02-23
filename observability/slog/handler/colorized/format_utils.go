// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package colorized

import (
	"log/slog"
	"strconv"
	"sync"
)

// Pre-allocated buffers for custom level formatting
var levelBufferPool = &sync.Pool{
	New: func() any {
		b := make([]byte, 0, 32)
		return &b
	},
}

// formatCustomLevel formats custom log levels without allocations
func formatCustomLevel(level slog.Level) string {
	buf := levelBufferPool.Get().(*[]byte) //nolint:errcheck
	defer func() {
		*buf = (*buf)[:0]
		levelBufferPool.Put(buf)
	}()

	switch {
	case level < slog.LevelInfo:
		*buf = append(*buf, "DBG"...)
		if diff := level - slog.LevelDebug; diff != 0 {
			if diff > 0 {
				*buf = append(*buf, '+')
			}
			*buf = strconv.AppendInt(*buf, int64(diff), 10)
		}
	case level < slog.LevelWarn:
		*buf = append(*buf, "INF"...)
		if diff := level - slog.LevelInfo; diff != 0 {
			if diff > 0 {
				*buf = append(*buf, '+')
			}
			*buf = strconv.AppendInt(*buf, int64(diff), 10)
		}
	case level < slog.LevelError:
		*buf = append(*buf, "WARN"...)
		if diff := level - slog.LevelWarn; diff != 0 {
			if diff > 0 {
				*buf = append(*buf, '+')
			}
			*buf = strconv.AppendInt(*buf, int64(diff), 10)
		}
	default:
		*buf = append(*buf, "ERR"...)
		if diff := level - slog.LevelError; diff != 0 {
			if diff > 0 {
				*buf = append(*buf, '+')
			}
			*buf = strconv.AppendInt(*buf, int64(diff), 10)
		}
	}

	return string(*buf)
}

// appendAny appends any value to buffer without allocations where possible
func appendAny(b []byte, v any) []byte {
	if v == nil {
		return append(b, "<nil>"...)
	}

	switch x := v.(type) {
	case bool:
		return strconv.AppendBool(b, x)
	case int:
		return strconv.AppendInt(b, int64(x), 10)
	case int8:
		return strconv.AppendInt(b, int64(x), 10)
	case int16:
		return strconv.AppendInt(b, int64(x), 10)
	case int32:
		return strconv.AppendInt(b, int64(x), 10)
	case int64:
		return strconv.AppendInt(b, x, 10)
	case uint:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint8:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint16:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint32:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint64:
		return strconv.AppendUint(b, x, 10)
	case float32:
		return strconv.AppendFloat(b, float64(x), 'g', -1, 32)
	case float64:
		return strconv.AppendFloat(b, x, 'g', -1, 64)
	case string:
		return append(b, x...)
	case []byte:
		return append(b, x...)
	default:
		// For complex types, we still need to allocate
		// but this is less common in typical logging
		return append(b, anyToStringFallback(v)...)
	}
}

// anyToStringFallback handles complex types that require allocation
// This is separated to make the common path more efficient
func anyToStringFallback(v any) string {
	// This will still allocate, but it's only for uncommon types
	switch x := v.(type) {
	case error:
		if x != nil {
			return x.Error()
		}
		return "<nil>"
	case []int:
		return formatIntSlice(x)
	case []string:
		return formatStringSlice(x)
	default:
		// Last resort - this allocates but is rare
		return strconv.Quote(formatDefault(v))
	}
}

// formatIntSlice formats int slice without fmt.Sprint
func formatIntSlice(s []int) string {
	if len(s) == 0 {
		return "[]"
	}

	buf := levelBufferPool.Get().(*[]byte) //nolint:errcheck
	defer func() {
		*buf = (*buf)[:0]
		levelBufferPool.Put(buf)
	}()

	*buf = append(*buf, '[')
	for i, v := range s {
		if i > 0 {
			*buf = append(*buf, ' ')
		}
		*buf = strconv.AppendInt(*buf, int64(v), 10)
	}
	*buf = append(*buf, ']')

	return string(*buf)
}

// formatStringSlice formats string slice without fmt.Sprint
func formatStringSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}

	buf := levelBufferPool.Get().(*[]byte) //nolint:errcheck
	defer func() {
		*buf = (*buf)[:0]
		levelBufferPool.Put(buf)
	}()

	*buf = append(*buf, '[')
	for i, v := range s {
		if i > 0 {
			*buf = append(*buf, ' ')
		}
		*buf = strconv.AppendQuote(*buf, v)
	}
	*buf = append(*buf, ']')

	return string(*buf)
}

// formatDefault is the absolute fallback - still better than fmt.Sprint
func formatDefault(v any) string {
	// Use a simple type switch for common types
	switch v.(type) {
	case nil:
		return "<nil>"
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		// These are handled by appendAny
		return ""
	default:
		// For truly unknown types, we need to allocate
		// but at least we avoid fmt.Sprint
		return "<unknown>"
	}
}
