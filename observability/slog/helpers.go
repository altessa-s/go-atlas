// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// ErrorKey is the attribute key used for error values in log records.
const ErrorKey string = "error"

// Error creates a slog.Attr for errors using the standard ErrorKey.
// Returns an empty attribute if err is nil, which slog ignores.
//
// Example:
//
//	logger.Error("failed", slogx.Error(err))
func Error(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.Any(ErrorKey, err)
}

// String creates a slog.Attr from a string or pointer to string.
// Returns an empty attribute (ignored by slog) if value is nil or empty.
//
// Example:
//
//	logger.Info("user", slogx.String("name", namePtr)) // nil-safe
func String[T interface{ ~string | ~*string }](key string, value T) slog.Attr {
	var val string
	switch v := any(value).(type) {
	case *string:
		if v == nil {
			return slog.Attr{}
		}
		val = *v
	case string:
		if v == "" {
			return slog.Attr{}
		}
		val = v
	}
	return slog.Attr{Key: key, Value: slog.StringValue(val)}
}

// Int creates a slog.Attr from an int or pointer to int.
// Returns an empty attribute (ignored by slog) if value is nil.
//
// Example:
//
//	logger.Debug("processed", slogx.Int("count", countPtr)) // nil-safe
func Int[T interface{ ~int | ~*int }](key string, value T) slog.Attr {
	var val int
	switch v := any(value).(type) {
	case int:
		val = v
	case *int:
		if v == nil {
			return slog.Attr{}
		}
		val = *v
	}
	return slog.Attr{Key: key, Value: slog.IntValue(val)}
}

// Int64 creates a slog.Attr from int64, int32, or their pointers.
// Returns an empty attribute (ignored by slog) if value is nil.
//
// Example:
//
//	logger.Info("found", slogx.Int64("user_id", userIDPtr)) // nil-safe
func Int64[T interface {
	~int64 | ~*int64 | ~int32 | ~*int32
}](key string, value T) slog.Attr {
	var val int64
	switch v := any(value).(type) {
	case int64:
		val = v
	case *int64:
		if v == nil {
			return slog.Attr{}
		}
		val = *v
	case *int32:
		if v == nil {
			return slog.Attr{}
		}
		val = int64(*v)
	case int32:
		val = int64(v)
	}
	return slog.Attr{Key: key, Value: slog.Int64Value(val)}
}

// ModuleKey is the attribute key used for identifying the subsystem or module.
const ModuleKey = "subsystem"

// Module creates a slog.Attr that identifies the subsystem generating the log.
//
// Example:
//
//	logger.Info("starting", slogx.Module("http-server"))
func Module(module string) slog.Attr {
	return slog.Attr{
		Key:   ModuleKey,
		Value: slog.StringValue(module),
	}
}

// ModuleM creates multiple module attributes as a slice for variadic slog methods.
//
// Example:
//
//	logger.Info("op", slogx.ModuleM("auth", "cache")...)
func ModuleM(module ...string) []any {
	return slices.Collect(coreslices.Map(module, func(m string) any {
		return slog.Attr{
			Key:   ModuleKey,
			Value: slog.StringValue(m),
		}
	}))
}

// UseDiscardLoggerAsDefault sets slog.DiscardHandler as the default logger.
// Useful for tests or temporarily disabling logging.
//
// Example:
//
//	slogx.UseDiscardLoggerAsDefault() // silences all default logger output
func UseDiscardLoggerAsDefault() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
}

// GlobalLevel is the default dynamic level used by loggers created via the factory package.
// Use [SetLevel] and [GetLevel] for convenient access.
var GlobalLevel = &slog.LevelVar{}

// SetLevel changes the [GlobalLevel] at runtime. Safe for concurrent use.
func SetLevel(level slog.Level) {
	GlobalLevel.Set(level)
}

// GetLevel returns the current [GlobalLevel]. Safe for concurrent use.
func GetLevel() slog.Level {
	return GlobalLevel.Level()
}

// MaskingReplaceAttr creates a ReplaceAttr function for slog handlers that masks sensitive fields.
// It is case-insensitive for keys.
func MaskingReplaceAttr(sensitiveTags []string, maskString string) func([]string, slog.Attr) slog.Attr {
	sensitiveSet := make(map[string]struct{}, len(sensitiveTags))
	for _, tag := range sensitiveTags {
		sensitiveSet[strings.ToLower(tag)] = struct{}{}
	}

	return func(_ []string, a slog.Attr) slog.Attr {
		key := strings.ToLower(a.Key)
		if _, ok := sensitiveSet[key]; ok {
			a.Value = slog.StringValue(maskString)
		}
		return a
	}
}

// HandlerWithShutdown is implemented by handlers that require graceful
// shutdown (e.g., the buffered handler). [Shutdown] traverses the handler
// chain looking for implementations of this interface.
type HandlerWithShutdown interface {
	Shutdown(ctx context.Context) error
}

// InnerHandler is implemented by middleware handlers that wrap another
// [slog.Handler]. [Shutdown] uses this to traverse the handler chain.
type InnerHandler interface {
	Inner() slog.Handler
}

// Shutdown attempts to gracefully shutdown a logger by traversing its handler chain.
// It looks for handlers implementing a Shutdown(context.Context) error method.
// The provided context controls the shutdown timeout; callers should set an
// appropriate deadline or timeout on it.
// The provided logger's handler is checked, and any wrapped handlers are recursively checked.
func Shutdown(ctx context.Context, l *slog.Logger) error {
	handler := l.Handler()
	return shutdownHandler(ctx, handler)
}

func shutdownHandler(ctx context.Context, h slog.Handler) error {
	// 1. Check if current handler supports shutdown
	if s, ok := h.(HandlerWithShutdown); ok {
		if err := s.Shutdown(ctx); err != nil {
			return err
		}
	}

	// 2. Check if it wraps another handler and traverse down
	if i, ok := h.(InnerHandler); ok {
		return shutdownHandler(ctx, i.Inner())
	}

	return nil
}
