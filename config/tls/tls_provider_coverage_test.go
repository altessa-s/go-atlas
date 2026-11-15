// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func providerFieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func tempFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	return p
}

func TestProviderFile_Validate(t *testing.T) {
	t.Parallel()

	cert, key := tempFile(t, "cert.pem"), tempFile(t, "key.pem")
	require.NoError(t, (&tlsconfig.ProviderFile{Certificate: cert, PrivateKey: key}).Validate())

	missing := filepath.Join(t.TempDir(), "missing.pem")
	verrs := providerFieldErrors(t, (&tlsconfig.ProviderFile{Certificate: missing, PrivateKey: missing}).Validate())
	require.Contains(t, verrs, "Certificate")
	require.Contains(t, verrs, "PrivateKey")
}

func TestProviderLetsEncrypt_Validate(t *testing.T) {
	t.Parallel()

	valid := tlsconfig.ProviderLetsEncrypt{Email: "ops@example.com", Domain: []string{"example.com"}, RenewBefore: time.Hour}
	require.NoError(t, valid.Validate())

	verrs := providerFieldErrors(t, (&tlsconfig.ProviderLetsEncrypt{}).Validate())
	require.Contains(t, verrs, "Domain")
	require.Contains(t, verrs, "Email")

	bad := valid
	bad.Email = "not-an-email"
	bad.Domain = []string{"bad domain!"}
	verrs = providerFieldErrors(t, bad.Validate())
	require.Contains(t, verrs, "Domain")
	require.Contains(t, verrs, "Email")

	internal := valid
	internal.StartInternalServer = true
	internal.ListenAddress = new("not an address")
	require.Contains(t, providerFieldErrors(t, internal.Validate()), "ListenAddress")

	internal.ListenAddress = new("0.0.0.0:8080")
	require.NoError(t, internal.Validate())
}

func validS3() tlsconfig.ProviderS3 {
	p := tlsconfig.ProviderS3{Bucket: "certs", CertificateKey: "tls.crt", PrivateKeyKey: "tls.key", PollInterval: time.Minute,
		Endpoint:  "http://localhost:9000",
		AccessKey: "ak",
		SecretKey: "sk",
		Region:    "us-east-1"}
	return p
}

func TestProviderS3_Validate(t *testing.T) {
	t.Parallel()

	p := validS3()
	require.NoError(t, p.Validate())

	verrs := providerFieldErrors(t, (&tlsconfig.ProviderS3{}).Validate())
	for _, f := range []string{"Bucket", "CertificateKey", "PrivateKeyKey"} {
		require.Contains(t, verrs, f)
	}

	withSSE := validS3()
	withSSE.SSE = &tlsconfig.ProviderS3SSE{Type: tlsconfig.SSETypeConfigKMS}
	require.NoError(t, withSSE.Validate())

	withSSE.SSE = &tlsconfig.ProviderS3SSE{Type: "aes"}
	require.Contains(t, providerFieldErrors(t, withSSE.Validate()), "SSE")
}

func TestProviderS3SSE_Validate(t *testing.T) {
	t.Parallel()

	require.Contains(t, providerFieldErrors(t, (&tlsconfig.ProviderS3SSE{}).Validate()), "Type")

	verrs := providerFieldErrors(t, (&tlsconfig.ProviderS3SSE{Type: tlsconfig.SSETypeConfigC}).Validate())
	require.Contains(t, verrs, "CustomerKey")
	require.Contains(t, verrs, "CustomerKeyMD5")

	require.NoError(t, (&tlsconfig.ProviderS3SSE{
		Type: tlsconfig.SSETypeConfigC, CustomerKey: "k", CustomerKeyMD5: "md5",
	}).Validate())
	require.NoError(t, (&tlsconfig.ProviderS3SSE{Type: tlsconfig.SSETypeConfigS3}).Validate())
}

func TestProviderVault_Validate(t *testing.T) {
	t.Parallel()

	valid := tlsconfig.ProviderVault{
		CommonName: "svc.example.com", Role: "svc",
		SubjectAlternativeNames: []string{"alt.example.com"}, IPSubjectAlternativeNames: []string{"10.0.0.1"},
	}
	require.NoError(t, valid.Validate())

	verrs := providerFieldErrors(t, (&tlsconfig.ProviderVault{}).Validate())
	require.Contains(t, verrs, "CommonName")
	require.Contains(t, verrs, "Role")

	bad := valid
	bad.SubjectAlternativeNames = []string{""}
	bad.IPSubjectAlternativeNames = []string{"not-an-ip"}
	verrs = providerFieldErrors(t, bad.Validate())
	require.Contains(t, verrs, "SubjectAlternativeNames")
	require.Contains(t, verrs, "IPSubjectAlternativeNames")
}

func TestProvider_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&tlsconfig.Provider{}).Validate())

	require.NoError(t, (&tlsconfig.Provider{
		Vault: &tlsconfig.ProviderVault{CommonName: "cn", Role: "r"},
	}).Validate())

	verrs := providerFieldErrors(t, (&tlsconfig.Provider{
		File:        &tlsconfig.ProviderFile{Certificate: filepath.Join(t.TempDir(), "nope")},
		LetsEncrypt: &tlsconfig.ProviderLetsEncrypt{},
		Vault:       &tlsconfig.ProviderVault{},
		S3:          &tlsconfig.ProviderS3{},
	}).Validate())
	for _, f := range []string{"File", "LetsEncrypt", "Vault", "S3"} {
		require.Contains(t, verrs, f)
	}
}

func TestClientAuth(t *testing.T) {
	t.Parallel()

	ca := tempFile(t, "ca.pem")
	c := tlsconfig.ClientAuth{CACerts: []string{ca}}
	c.Normalize()
	require.Equal(t, []string{ca}, c.CACerts, "absolute paths are kept")
	require.NoError(t, c.Validate())

	bad := tlsconfig.ClientAuth{CACerts: []string{filepath.Join(t.TempDir(), "missing.pem")}}
	require.Contains(t, providerFieldErrors(t, bad.Validate()), "CACerts")
}

func TestServer_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&tlsconfig.Server{Provider: tlsconfig.ProviderTypeS3, MinTLSVersion: "1.3"}).Validate())

	verrs := providerFieldErrors(t, (&tlsconfig.Server{Provider: "acme", MinTLSVersion: "1.0"}).Validate())
	require.Contains(t, verrs, "Provider")
	require.Contains(t, verrs, "MinTLSVersion")

	require.Contains(t, providerFieldErrors(t, (&tlsconfig.Server{}).Validate()), "MinTLSVersion")
}

// TestProviderS3_ValidatesEmbeddedS3Config pins that the embedded
// s3config.Config, whose Validate has a pointer receiver, is validated.
func TestProviderS3_ValidatesEmbeddedS3Config(t *testing.T) {
	t.Parallel()

	p := validS3()
	p.Endpoint = ""
	p.AccessKey = ""
	// The embedded block's errors are inlined, matching its inline YAML keys.
	verrs := providerFieldErrors(t, p.Validate())
	require.Contains(t, verrs, "Endpoint")
	require.Contains(t, verrs, "AccessKey")
}
