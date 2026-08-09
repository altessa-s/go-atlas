// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

// referenceSecret is the value every rendering is compared against. Its content
// is irrelevant — what matters is that it differs from whatever the fuzzer
// supplies, so any renderer whose output depends on the secret shows up as a
// difference.
const referenceSecret = "reference-secret-value"

// renderings is every way this package turns a RedactedString into bytes.
// Keeping them in one table is deliberate: a renderer added without a redaction
// step is only caught if the table is the place people add renderers to.
func renderings(s redacted.RedactedString) map[string]func() (string, error) {
	return map[string]func() (string, error){
		"String":   func() (string, error) { return s.String(), nil },
		"GoString": func() (string, error) { return s.GoString(), nil },
		"fmt %v":   func() (string, error) { return fmt.Sprintf("%v", s), nil },
		"fmt %s":   func() (string, error) { return fmt.Sprintf("%s", s), nil },
		"fmt %q":   func() (string, error) { return fmt.Sprintf("%q", s), nil },
		"fmt %#v":  func() (string, error) { return fmt.Sprintf("%#v", s), nil },
		"slog":     func() (string, error) { return s.LogValue().String(), nil },
		"json": func() (string, error) {
			b, err := json.Marshal(s)
			return string(b), err
		},
		"yaml": func() (string, error) {
			b, err := yaml.Marshal(s)
			return string(b), err
		},
		"text": func() (string, error) {
			b, err := s.MarshalText()
			return string(b), err
		},
		"bson": func() (string, error) {
			_, raw, err := s.MarshalBSONValue()
			return string(raw), err
		},
		"slog handler": func() (string, error) {
			var sb strings.Builder
			// The timestamp is dropped: two renderings taken microseconds apart
			// would otherwise differ for a reason that has nothing to do with
			// the secret, and this table's whole point is that a difference
			// means a leak.
			opts := &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			}}
			slog.New(slog.NewTextHandler(&sb, opts)).Info("msg", slog.Any("secret", s))
			return sb.String(), nil
		},
	}
}

// FuzzNoRendererDependsOnTheSecret is the type's entire reason to exist, stated
// as one property: what a RedactedString renders to must not depend on what it
// holds.
//
// Comparing against a fixed reference secret rather than searching the output
// for the fuzzed one is what makes this usable. A containment test has to
// exclude the cases where a correct rendering trivially contains the input —
// the secret "0" is inside the JSON-escaped placeholder "<redacted>",
// and the secret "<redacted>" is the placeholder — and every such exclusion is
// a hole the real thing could hide in. Two renderings that must be byte-equal
// have no such holes: if any path forwards the value, they differ.
//
// The surface is wide (JSON, YAML, Text, BSON, Stringer, GoStringer, slog, and
// every fmt verb) and it takes one forgetful path for a credential to reach a
// log aggregator.
func FuzzNoRendererDependsOnTheSecret(f *testing.F) {
	f.Add("hunter2")
	f.Add("")
	f.Add("<redacted>") // A secret that is the placeholder.
	f.Add("0")          // A secret inside the JSON escape of the placeholder.
	f.Add(`{"a":"b"}`)
	f.Add("line\nbreak")
	f.Add(`"quoted"`)
	f.Add("%s %v %#v")
	f.Add(strings.Repeat("A", 1024))
	f.Add("\x00\xff")

	reference := renderings(redacted.RedactedString(referenceSecret))

	f.Fuzz(func(t *testing.T, secret string) {
		got := renderings(redacted.RedactedString(secret))

		for name, render := range got {
			out, err := render()
			require.NoError(t, err, "%s failed to render", name)

			want, err := reference[name]()
			require.NoError(t, err, "%s failed to render the reference", name)

			require.Equal(t, want, out,
				"%s renders differently for different secrets, so it forwards part of the value", name)
		}

		// Expose is the one deliberate way out, and it must keep working — a
		// type that redacts even its own accessor is unusable.
		require.Equal(t, secret, redacted.RedactedString(secret).Expose())
	})
}

// FuzzUnmarshalIsTransparent pins the other half of the contract: decoding a
// RedactedString must land on exactly what decoding a plain string would.
//
// Round-tripping is deliberately asymmetric — marshaling redacts, so the secret
// goes in through a decoder and comes back only via Expose. Comparing against
// the plain-string decode rather than against the original input is what keeps
// the oracle about this type: JSON and BSON substitute U+FFFD for bytes that
// are not UTF-8, and yaml.v3 does not round-trip a string of only newlines at
// all. Those are the encodings' behavior, and the wrapper's job is to inherit
// them, not to improve on them. A decoder that mangled the value beyond that
// would leave a service authenticating with a corrupted credential — a failure
// that surfaces at the remote end as a permission error, nowhere near the
// config that caused it.
func FuzzUnmarshalIsTransparent(f *testing.F) {
	f.Add("hunter2")
	f.Add("")
	f.Add("<redacted>")
	f.Add("line\nbreak")
	f.Add("\n") // yaml.v3 does not round-trip this, for plain strings either.
	f.Add(`"quoted"`)
	f.Add("  leading and trailing  ")
	f.Add("тайна")
	f.Add("\xa5") // Not UTF-8: the text path must still carry it.

	f.Fuzz(func(t *testing.T, secret string) {
		// MarshalText/UnmarshalText is the byte-transparent path with no
		// encoder in front of it, so it carries anything.
		var fromText redacted.RedactedString
		require.NoError(t, fromText.UnmarshalText([]byte(secret)))
		require.Equal(t, secret, fromText.Expose())

		t.Run("json", func(t *testing.T) {
			encoded, err := json.Marshal(secret) // As a config file would hold it.
			require.NoError(t, err)

			assertTransparent(t,
				func(v *string) error { return json.Unmarshal(encoded, v) },
				func(v *redacted.RedactedString) error { return json.Unmarshal(encoded, v) })
		})

		t.Run("yaml", func(t *testing.T) {
			encoded, err := yaml.Marshal(secret)
			require.NoError(t, err)

			assertTransparent(t,
				func(v *string) error { return yaml.Unmarshal(encoded, v) },
				func(v *redacted.RedactedString) error { return yaml.Unmarshal(encoded, v) })
		})

		t.Run("bson", func(t *testing.T) {
			typ, raw, err := bson.MarshalValue(secret)
			require.NoError(t, err)

			assertTransparent(t,
				func(v *string) error { return bson.UnmarshalValue(typ, raw, v) },
				func(v *redacted.RedactedString) error { return v.UnmarshalBSONValue(byte(typ), raw) })
		})
	})
}

// assertTransparent requires that decoding into a RedactedString behaves
// exactly like decoding into a plain string: the same value on success, and a
// failure whenever the plain decode fails.
//
// Accepting "both failed" as agreement is not a loophole, it is the contract.
// yaml.v3 emits YAML it cannot read back for a string containing a tab, and no
// wrapper can fix that from the inside; what a wrapper must not do is diverge
// from the encoding it wraps — succeeding where a string fails, or the reverse,
// is how a config value silently becomes something else.
func assertTransparent(
	t *testing.T,
	decodePlain func(*string) error,
	decodeRedacted func(*redacted.RedactedString) error,
) {
	t.Helper()

	var plain string
	plainErr := decodePlain(&plain)

	var decoded redacted.RedactedString
	decodedErr := decodeRedacted(&decoded)

	if plainErr != nil {
		require.Error(t, decodedErr,
			"decoding a plain string failed (%v) but the redacted one succeeded with %q", plainErr, decoded.Expose())
		return
	}

	require.NoError(t, decodedErr, "decoding a plain string succeeded but the redacted one failed")
	require.Equal(t, plain, decoded.Expose())
}
