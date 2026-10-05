// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redacted provides RedactedString, a named string type for
// credential and other sensitive fields that substitutes its content
// with a fixed placeholder in every standard output context (fmt,
// log/slog, encoding/json, gopkg.in/yaml.v3, encoding.TextMarshaler,
// go.mongodb.org/mongo-driver/v2/bson). The underlying plain text is
// only reachable through the explicit Expose accessor.
//
// # Why a named string
//
// RedactedString keeps reflect.Kind == reflect.String. Decoders that
// honor the unmarshal interfaces call its methods: gopkg.in/yaml.v3
// invokes UnmarshalYAML, the Mongo v2 BSON driver UnmarshalBSONValue,
// and encoding/json UnmarshalJSON. Code that sets fields purely through
// reflection bypasses them, which is safe because the underlying kind
// is string. Marshal emits <redacted>; Unmarshal restores the plain
// underlying value.
//
// # Usage
//
//	type DatabaseConfig struct {
//	    URI redacted.RedactedString `yaml:"uri" json:"uri" bson:"uri"`
//	}
//
//	cfg := DatabaseConfig{URI: "mongodb://user:pass@host/db"}
//	slog.Info("connecting", "cfg", cfg)
//	// level=INFO msg=connecting cfg.uri=<redacted>
//
//	conn := cfg.URI.Expose() // intentional, plain text
//
// # When NOT to use
//
// For credentials that must be zeroed from memory after use, reach
// for core/text/strings.SecureString instead. RedactedString is a
// cheap log/marshal guard, not a secure-memory primitive; the
// SecureString method on RedactedString is provided as a one-shot
// bridge for callers that need both behaviors.
//
// # Serialization
//
// RedactedString implements json.Marshaler / json.Unmarshaler,
// the YAML marshaler / callback unmarshaler, encoding.TextMarshaler /
// encoding.TextUnmarshaler and bson.ValueMarshaler /
// bson.ValueUnmarshaler. Marshal always emits the placeholder
// "<redacted>"; Unmarshal stores the incoming string verbatim into
// the underlying value. IsZero returns true for the empty string,
// letting `bson:",omitempty"` strip empty fields entirely.
//
// This package has no external production dependencies. The fixed BSON string
// redaction hook uses only the standard library and works without a custom
// registry. YAML decoding uses the callback interface supported by yaml.v3.
package redacted
