// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package colorized

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
)

func TestHandler_QuotesUntrustedKeys(t *testing.T) {
	// Serial: rendering depends on process-global color.NoColor changed by sibling tests.
	for _, key := range []string{"line\nforged", "carriage\rreturn", "ansi\x1b[31m", "nul\x00key", "two words", "fake=value", "quote\"key", "bad\xffutf8"} {
		t.Run(strconv.Quote(key), func(t *testing.T) {
			for _, mode := range []string{"record", "bound", "group", "bound group", "nested"} {
				t.Run(mode, func(t *testing.T) {
					var out bytes.Buffer
					var h slog.Handler = NewHandler(&out, WithLevel(slog.LevelDebug))
					rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
					expected := key
					switch mode {
					case "record":
						rec.AddAttrs(slog.String(key, "value"))
					case "bound":
						h = h.WithAttrs([]slog.Attr{slog.String(key, "value")})
					case "group":
						h = h.WithGroup(key)
						rec.AddAttrs(slog.String("leaf", "value"))
						expected += ".leaf"
					case "bound group":
						h = h.WithGroup(key).WithAttrs([]slog.Attr{slog.String("leaf", "value")})
						expected += ".leaf"
					case "nested":
						rec.AddAttrs(slog.Group(key, slog.String("leaf", "value")))
						expected += ".leaf"
					}
					require.NoError(t, h.Handle(t.Context(), rec))
					require.Contains(t, out.String(), strconv.Quote(expected)+"=value")
					require.Equal(t, 1, strings.Count(out.String(), "\n"))
					require.NotContains(t, out.String(), "\r")
					require.NotContains(t, out.String(), "\x1b")
				})
			}
		})
	}
}

func TestHandler_QuotesKeysWithANSI(t *testing.T) {
	// Serial: changes process-global color setting and cache, as existing color tests do.
	t.Setenv("NO_COLOR", "")
	oldNoColor, oldCache := color.NoColor, globalColorCache
	color.NoColor = false
	globalColorCache = &colorCacheAdapter{newColorCache()}
	t.Cleanup(func() { color.NoColor = oldNoColor; globalColorCache = oldCache })
	var out bytes.Buffer
	h := NewHandler(&out, WithLevel(slog.LevelDebug)).WithGroup("group\nforged")
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	rec.AddAttrs(slog.String("key\x1b[31m", "value"))
	require.NoError(t, h.Handle(t.Context(), rec))
	require.Contains(t, out.String(), "\x1b[")
	require.Contains(t, out.String(), strconv.Quote("group\nforged.key\x1b[31m"))
	require.Equal(t, 1, strings.Count(out.String(), "\n"))
}
