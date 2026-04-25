// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"log/slog"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const redacted = "<redacted>"

// Secret is a named string type for credential fields that redacts its value
// in all common output contexts (fmt, JSON, YAML, slog) to prevent accidental exposure.
//
// Because Secret has underlying kind reflect.String, the config loader infrastructure
// (secret expander, reflection setter, YAML v3 decoder) works without modification.
//
// Use Expose() when the plain-text value is intentionally needed (e.g., passing to a client library).
type Secret string

// String implements fmt.Stringer, returning a redacted placeholder.
func (s Secret) String() string { return redacted }

// GoString implements fmt.GoStringer for %#v formatting.
func (s Secret) GoString() string { return "Secret{" + redacted + "}" }

// MarshalJSON implements json.Marshaler, returning a redacted JSON string.
func (s Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

// MarshalYAML implements yaml.Marshaler, returning a redacted value.
func (s Secret) MarshalYAML() (any, error) {
	return redacted, nil
}

// MarshalText implements encoding.TextMarshaler, returning a redacted value.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}

// LogValue implements slog.LogValuer, returning a redacted log value.
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(redacted)
}

// Expose returns the underlying plain-text credential.
// Use this only when the actual value is required (e.g., passing to a client library).
func (s Secret) Expose() string { return string(s) }

// IsEmpty reports whether the secret is empty.
func (s Secret) IsEmpty() bool { return s == "" }

// SecureString converts the secret into a pooled SecureString with memory zeroing.
// The caller must call Clear() on the returned value when done.
func (s Secret) SecureString() *corestrings.SecureString {
	return corestrings.NewSecureString(string(s))
}
