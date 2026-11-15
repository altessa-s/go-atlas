// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kms_test

import (
	"crypto/tls"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/mongo/kms"
)

type fakeProvider struct {
	key kms.Key
	tls *tls.Config
}

func (fakeProvider) Name() string                 { return "fake" }
func (fakeProvider) Credentials() kms.Credentials { return kms.Credentials{} }
func (fakeProvider) Clear()                       {}
func (p fakeProvider) MasterKey() kms.Key         { return p.key }
func (p fakeProvider) TLSConfig() *tls.Config     { return p.tls }

func TestCapabilityHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		p               fakeProvider
		wantKey, wantTL bool
	}{
		{name: "none", p: fakeProvider{}},
		{name: "key only", p: fakeProvider{key: kms.Key{"k": "v"}}, wantKey: true},
		{name: "tls only", p: fakeProvider{tls: &tls.Config{MinVersion: tls.VersionTLS13}}, wantTL: true},
		{name: "both", p: fakeProvider{key: kms.Key{}, tls: &tls.Config{MinVersion: tls.VersionTLS13}}, wantKey: true, wantTL: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.wantKey, kms.HasMasterKey(tc.p))
			require.Equal(t, tc.wantTL, kms.HasCustomTLS(tc.p))
			require.Equal(t, tc.wantKey && tc.wantTL, kms.IsFullyFeatured(tc.p))
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	withValue := kms.ValidationError{Provider: "local", Field: "key", Value: "x", Reason: "bad"}
	require.Equal(t, `validation failed for provider local field key (value: "x"): bad`, withValue.Error())
	require.ErrorIs(t, withValue, kms.ErrConfigurationError)

	lenErr := kms.NewKeyLengthError("local", 10, 96)
	require.ErrorIs(t, lenErr, kms.ErrConfigurationError)
	ve, ok := errors.AsType[kms.ValidationError](lenErr)
	require.True(t, ok)
	require.Equal(t, "masterKey", ve.Field)
	require.Equal(t, "validation failed for provider local field masterKey: invalid length: got 10 bytes, expected 96 bytes",
		lenErr.Error())

	missing := kms.NewMissingCredentialError("aws", "accessKeyId")
	ve, ok = errors.AsType[kms.ValidationError](missing)
	require.True(t, ok)
	require.Equal(t, "aws", ve.Provider)
	require.Equal(t, "accessKeyId", ve.Field)
	require.ErrorIs(t, missing, kms.ErrConfigurationError)
}

func TestCredentialError(t *testing.T) {
	t.Parallel()

	plain := kms.CredentialError{Provider: "aws", Operation: "retrieve", Reason: "expired"}
	require.Equal(t, "credential retrieve failed for provider aws: expired", plain.Error())
	require.ErrorIs(t, plain, kms.ErrInvalidCredentials)

	cause := errors.New("boom")
	wrapped := kms.CredentialError{Provider: "aws", Operation: "store", Reason: "io", Underlying: cause}
	require.Equal(t, "credential store failed for provider aws: io: boom", wrapped.Error())
	require.ErrorIs(t, wrapped, cause)
	require.NotErrorIs(t, wrapped, kms.ErrInvalidCredentials)
}

func TestKeyError(t *testing.T) {
	t.Parallel()

	plain := kms.KeyError{Provider: "gcp", Operation: "decrypt", KeyID: "k1", Reason: "denied"}
	require.Equal(t, `key decrypt failed for provider gcp key "k1": denied`, plain.Error())
	require.ErrorIs(t, plain, kms.ErrInvalidKey)

	cause := errors.New("boom")
	wrapped := kms.KeyError{Provider: "gcp", Operation: "encrypt", KeyID: "k2", Reason: "io", Underlying: cause}
	require.Equal(t, `key encrypt failed for provider gcp key "k2": io: boom`, wrapped.Error())
	require.ErrorIs(t, wrapped, cause)
	require.NotErrorIs(t, wrapped, kms.ErrInvalidKey)
}
