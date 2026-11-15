// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pagingconfig

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// KeyEncoding names how a configured signing key is turned into key bytes.
type KeyEncoding string

const (
	// KeyEncodingRaw uses the bytes of the configured string as they are.
	KeyEncodingRaw KeyEncoding = "raw"
	// KeyEncodingBase64 decodes the configured string as standard base64
	// (RFC 4648, padded); line breaks inside the value are ignored.
	KeyEncodingBase64 KeyEncoding = "base64"
)

const (
	// MinKeyLength is the shortest signing key accepted, in bytes after
	// decoding. It mirrors pagetoken.MinKeyLength; schemas do not import runtime
	// packages.
	MinKeyLength = 32

	// DefaultTokenTTL is how long a page token stays valid unless configured.
	// It mirrors pagetoken.DefaultTTL.
	DefaultTokenTTL = 24 * time.Hour
)

var (
	// ErrKeyEncoding reports a key that does not decode under its encoding,
	// or an unknown encoding.
	ErrKeyEncoding = errors.New("signing key does not decode")
	// ErrKeyTooShort reports a key shorter than MinKeyLength bytes after
	// decoding.
	ErrKeyTooShort = fmt.Errorf("signing key must be at least %d bytes", MinKeyLength)

	// ErrStrictWithoutEncryption reports encryption.strict set without
	// encryption.enabled.
	ErrStrictWithoutEncryption = errors.New("strict requires enabled")

	// errNotAKey reports KeyRule applied to a value that is not a
	// redacted.RedactedString.
	errNotAKey = errors.New("must be a redacted signing key")
)

// Config configures signed page tokens (page-token codecs): the current signing
// key, the retired keys that still verify tokens issued before a rotation, how
// the keys are encoded, how long a token stays valid, and whether tokens are
// encrypted.
type Config struct {
	// SigningKey signs page tokens: at least MinKeyLength bytes after
	// decoding, the same on every replica.
	SigningKey redacted.RedactedString `yaml:"signingKey"`

	// PreviousKeys still verify tokens signed before a key rotation. Keep a
	// retired key for one TokenTTL after the rotation.
	PreviousKeys []redacted.RedactedString `yaml:"previousKeys"`

	// KeyEncoding is how SigningKey and PreviousKeys are encoded: raw (the
	// string bytes) or base64. Empty means raw.
	KeyEncoding KeyEncoding `yaml:"keyEncoding" default:"raw"`

	// TokenTTL is how long a page token stays valid; 0 keeps the codec
	// default (24h).
	TokenTTL time.Duration `yaml:"tokenTTL" default:"24h"`

	// Encryption issues encrypted page tokens instead of signed ones.
	Encryption Encryption `yaml:"encryption"`
}

// Encryption configures encrypted page tokens, whose payload and bindings the
// client cannot read. The encryption keys are derived from the signing keys,
// so they rotate with them. Every codec opens encrypted tokens, so a rollout
// enables encryption replica by replica, then sets Strict once every signed
// token in circulation has expired.
type Encryption struct {
	// Enabled issues encrypted tokens; signed tokens are still accepted.
	Enabled bool `yaml:"enabled"`

	// Strict also rejects signed tokens. It requires Enabled.
	Strict bool `yaml:"strict"`
}

// Validate requires a signing key, rejects an empty previous key, and checks
// that every key decodes to at least MinKeyLength bytes under KeyEncoding,
// that TokenTTL is not negative, and that encryption.strict is set only
// with encryption.enabled.
func (c *Config) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.KeyEncoding, validation.In(KeyEncodingRaw, KeyEncodingBase64)),
		validation.Field(&c.SigningKey, validation.Required, KeyRule(c.KeyEncoding)),
		validation.Field(&c.PreviousKeys, validation.Each(validation.Required, KeyRule(c.KeyEncoding))),
		validation.Field(&c.TokenTTL, validation.Min(time.Duration(0))),
		validation.Field(&c.Encryption),
	)
}

// Validate rejects Strict without Enabled.
func (e Encryption) Validate() error {
	return validationconfig.ValidateStruct(&e,
		validation.Field(&e.Strict, validation.When(e.Strict && !e.Enabled, validation.By(func(any) error {
			return ErrStrictWithoutEncryption
		}))),
	)
}

// Keys returns the decoded signing key and previous keys. It does not check
// their length: Validate does, and pagetoken.New rejects a short key with
// pagetoken.ErrWeakKey.
func (c *Config) Keys() (current []byte, previous [][]byte, err error) {
	if current, err = DecodeKey(c.KeyEncoding, c.SigningKey); err != nil {
		return nil, nil, coreerrs.Wrap(err, "signingKey")
	}
	previous = make([][]byte, 0, len(c.PreviousKeys))
	for i, k := range c.PreviousKeys {
		b, err := DecodeKey(c.KeyEncoding, k)
		if err != nil {
			return nil, nil, coreerrs.Wrapf(err, "previousKeys[%d]", i)
		}
		previous = append(previous, b)
	}
	return current, previous, nil
}

// DecodeKey turns a configured key into key bytes under enc (empty means
// raw). Errors never quote the key.
func DecodeKey(enc KeyEncoding, key redacted.RedactedString) ([]byte, error) {
	switch enc {
	case "", KeyEncodingRaw:
		return []byte(key.Expose()), nil
	case KeyEncodingBase64:
		b, err := base64.StdEncoding.DecodeString(key.Expose())
		if err != nil {
			return nil, ErrKeyEncoding
		}
		return b, nil
	default:
		return nil, ErrKeyEncoding
	}
}

// KeyRule returns a validation rule that accepts an empty key and otherwise
// requires it to decode under enc to at least MinKeyLength bytes. Schemas
// that keep signing keys under their own field names validate them with it.
func KeyRule(enc KeyEncoding) validation.Rule {
	return validation.By(func(value any) error {
		key, ok := value.(redacted.RedactedString)
		if !ok {
			return errNotAKey
		}
		if key.IsEmpty() {
			return nil
		}
		b, err := DecodeKey(enc, key)
		if err != nil {
			return err
		}
		if len(b) < MinKeyLength {
			return ErrKeyTooShort
		}
		return nil
	})
}
