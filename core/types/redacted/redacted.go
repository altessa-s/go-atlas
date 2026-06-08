// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted

import (
	"log/slog"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// placeholder is the fixed token RedactedString emits in every redacting
// output path. It is intentionally a package-level constant so the value
// is identical across String, GoString, LogValue, MarshalJSON, MarshalYAML,
// MarshalText and MarshalBSONValue.
const placeholder = "<redacted>"

// RedactedString is a named string type for credential and other
// sensitive fields. Every standard output path (fmt, log/slog,
// encoding/json, gopkg.in/yaml.v3, encoding.TextMarshaler, BSON v2)
// substitutes the contained value with the fixed placeholder
// "<redacted>". The underlying plain text is reachable only through
// Expose.
//
// RedactedString keeps reflect.Kind == reflect.String so loaders that
// assign string scalars via reflection (yaml.v3, the repo config
// loader, the Mongo v2 BSON driver) populate it without invoking any
// of the Unmarshal* methods. The Unmarshal* methods exist anyway to
// document and guarantee the symmetric round-trip contract for code
// paths that do go through interface dispatch.
type RedactedString string

// String implements fmt.Stringer, returning the redacted placeholder.
func (s RedactedString) String() string { return placeholder }

// GoString implements fmt.GoStringer for %#v formatting.
func (s RedactedString) GoString() string { return "RedactedString{" + placeholder + "}" }

// LogValue implements slog.LogValuer, returning the redacted placeholder
// as a slog string value.
func (s RedactedString) LogValue() slog.Value {
	return slog.StringValue(placeholder)
}

// Expose returns the underlying plain-text value. Use this only when
// the actual value is required (passing to a client library, computing
// a hash, etc.) — never to log or serialize the secret.
func (s RedactedString) Expose() string { return string(s) }

// IsEmpty reports whether the underlying value is the empty string.
func (s RedactedString) IsEmpty() bool { return s == "" }

// IsZero reports whether the underlying value is the empty string.
// It is the bson:",omitempty" hook: empty RedactedString fields are
// stripped entirely from the on-the-wire document.
func (s RedactedString) IsZero() bool { return s == "" }

// SecureString converts the redacted value into a pooled SecureString
// with memory zeroing. The caller must call Clear() on the returned
// value when done.
func (s RedactedString) SecureString() *corestrings.SecureString {
	return corestrings.NewSecureString(string(s))
}
