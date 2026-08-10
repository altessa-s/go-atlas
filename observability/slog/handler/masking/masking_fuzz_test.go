// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

// referenceSecret is what every rendering is compared against. Its content is
// irrelevant; what matters is that it differs from whatever the fuzzer supplies,
// so a handler that forwards any part of the value shows up as a difference.
const referenceSecret = "reference-secret-value"

// nested is a struct carrying a sensitive field, so the reflection walk — not
// just the top-level attribute path — is exercised.
type nested struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Inner    *nested
}

// render logs one attribute through the masking handler and returns the text a
// collector would receive. The timestamp is dropped: two renderings taken
// microseconds apart would otherwise differ for a reason unrelated to the
// secret, and a difference is exactly what this file reads as a leak.
func render(tb testing.TB, build func(secret string) slog.Attr, secret string) string {
	tb.Helper()

	var sb strings.Builder
	inner := slog.NewTextHandler(&sb, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})

	handler := masking.NewHandler(inner,
		masking.WithDefaults(),
		masking.WithMaskNestedFields(),
		masking.WithField("password", masking.FullMask()),
	)

	slog.New(handler).LogAttrs(context.Background(), slog.LevelInfo, "msg", build(secret))

	return sb.String()
}

// FuzzMaskedFieldsDoNotReachTheOutput is the leak oracle, stated as
// independence: what the handler emits for a masked field must not depend on
// what that field held.
//
// Comparing against a fixed reference rather than searching the output for the
// fuzzed secret is what makes this usable — a containment test has to excuse
// every case where a correct rendering trivially contains the input (a
// one-character secret is inside the level, the message, the mask characters),
// and each excuse is a hole the real thing could hide in. Two renderings that
// must be byte-equal have no such holes.
//
// The shapes below are the paths a value can take into a log line: a plain
// attribute, a group, a struct walked by reflection, and a pointer cycle that
// the depth guard has to stop without dropping the masking on the way.
func FuzzMaskedFieldsDoNotReachTheOutput(f *testing.F) {
	f.Add("hunter2", uint8(0))
	f.Add("", uint8(1))
	f.Add("****", uint8(2))
	f.Add("a", uint8(3))
	f.Add(strings.Repeat("A", 512), uint8(0))
	f.Add("line\nbreak", uint8(1))
	f.Add("\x00\xff", uint8(2))

	shapes := []func(secret string) slog.Attr{
		// A plain attribute under a masked key.
		func(secret string) slog.Attr { return slog.String("password", secret) },
		// The same key inside a group.
		func(secret string) slog.Attr {
			return slog.Group("credentials", slog.String("password", secret))
		},
		// A struct field, reached by the reflection walk.
		func(secret string) slog.Attr {
			return slog.Any("account", nested{User: "svc", Password: secret})
		},
		// A self-referencing struct: the walk must stop and still mask.
		func(secret string) slog.Attr {
			n := &nested{User: "svc", Password: secret}
			n.Inner = n
			return slog.Any("account", n)
		},
	}

	f.Fuzz(func(t *testing.T, secret string, shapeSel uint8) {
		if secret == "" {
			// A mask of "" is "", so an empty secret renders differently from a
			// non-empty one by construction. Nothing leaks — the line reveals
			// only that the field was empty, which is what an absent value
			// looks like anyway.
			t.Skip("an empty secret is indistinguishable from an empty rendering")
		}

		shape := shapes[int(shapeSel)%len(shapes)]

		require.Equal(t, render(t, shape, referenceSecret), render(t, shape, secret),
			"the log line depends on the masked value, so part of it is being forwarded")
	})
}

// FuzzUnmaskedFieldsAreUntouched pins the other half: masking must not swallow
// the fields nobody asked to hide.
//
// A handler that masked too much would be safe and useless, and the failure is
// quiet — the field is simply missing from the line when someone needs it. It
// also guards the pattern matcher from the opposite mistake to the one above:
// a pattern loose enough to catch everything.
func FuzzUnmaskedFieldsAreUntouched(f *testing.F) {
	f.Add("order-1")
	f.Add("")
	f.Add("password") // The word, as a value rather than a key.
	f.Add("a=b c=d")
	f.Add("значение")

	f.Fuzz(func(t *testing.T, value string) {
		if value == "" {
			t.Skip("an empty value is indistinguishable from an omitted one")
		}

		out := render(t, func(v string) slog.Attr { return slog.String("order_id", v) }, value)

		// slog quotes a value containing spaces or control characters, so the
		// comparison is against what the inner handler would have written for
		// the same attribute — not against the raw string.
		var sb strings.Builder
		slog.New(slog.NewTextHandler(&sb, &slog.HandlerOptions{
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			},
		})).LogAttrs(context.Background(), slog.LevelInfo, "msg", slog.String("order_id", value))

		require.Equal(t, sb.String(), out,
			"a field nobody asked to mask was altered: %q", value)
	})
}
