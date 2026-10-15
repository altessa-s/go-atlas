// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls"
)

// oidTitle is the X.520 title attribute — not one of the pkix.Name fields.
var oidTitle = asn1.ObjectIdentifier{2, 5, 4, 12}

// fullDN carries every pkix.Name field plus an extra attribute.
func fullDN() pkix.Name {
	return pkix.Name{
		CommonName:         "billing",
		SerialNumber:       "SN-1",
		Organization:       []string{"Acme"},
		OrganizationalUnit: []string{"Payments"},
		Country:            []string{"US"},
		Locality:           []string{"Springfield"},
		Province:           []string{"Oregon"},
		StreetAddress:      []string{"742 Evergreen Terrace"},
		PostalCode:         []string{"97403"},
		ExtraNames:         []pkix.AttributeTypeAndValue{{Type: oidTitle, Value: "service"}},
	}
}

// parsedDNCert issues a self-signed certificate with dn as both Subject and
// Issuer and parses it back, so Subject.Names / Issuer.Names are populated as
// in a real handshake.
func parsedDNCert(t *testing.T, dn pkix.Name) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      dn,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func TestDNValidatorsCompareEveryField(t *testing.T) {
	t.Parallel()
	cert := parsedDNCert(t, fullDN())

	tests := []struct {
		name  string
		want  pkix.Name
		match bool
	}{
		{"all fields match", fullDN(), true},
		{"parsed DN copied from the certificate", cert.Subject, true},
		{"locality only", pkix.Name{Locality: []string{"Springfield"}}, true},
		{"locality mismatch", pkix.Name{CommonName: "billing", Locality: []string{"Shelbyville"}}, false},
		{"province mismatch", pkix.Name{CommonName: "billing", Province: []string{"Ohio"}}, false},
		{"street mismatch", pkix.Name{CommonName: "billing", StreetAddress: []string{"1 Elm St"}}, false},
		{"postal code mismatch", pkix.Name{CommonName: "billing", PostalCode: []string{"00000"}}, false},
		{"extra attribute match", pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{{Type: oidTitle, Value: "service"}}}, true},
		{
			"extra attribute mismatch",
			pkix.Name{CommonName: "billing", ExtraNames: []pkix.AttributeTypeAndValue{{Type: oidTitle, Value: "admin"}}},
			false,
		},
		{
			"names-only attribute mismatch",
			pkix.Name{Names: []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{2, 5, 4, 7}, Value: "Shelbyville"}}},
			false,
		},
		{
			"names-only attribute match",
			pkix.Name{Names: []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{2, 5, 4, 7}, Value: "Springfield"}}},
			true,
		},
		{
			"names entry conflicting with the struct field",
			pkix.Name{CommonName: "billing", Names: []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "other"}}},
			false,
		},
		{
			"extra attribute absent from certificate",
			pkix.Name{CommonName: "billing", ExtraNames: []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "Bart"}}},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			subjErr := mtls.SubjectValidator(tc.want)(cert)
			issErr := mtls.IssuerValidator(tc.want)(cert)
			if tc.match {
				require.NoError(t, subjErr)
				require.NoError(t, issErr)
				return
			}
			require.ErrorIs(t, subjErr, mtls.ErrSubjectMismatch)
			require.ErrorIs(t, issErr, mtls.ErrIssuerMismatch)
		})
	}
}

func TestDNValidatorsRejectUncheckableAttribute(t *testing.T) {
	t.Parallel()
	cert := parsedDNCert(t, fullDN())
	unsupported := pkix.Name{
		CommonName: "billing",
		ExtraNames: []pkix.AttributeTypeAndValue{{Type: oidTitle, Value: 42}},
	}

	// A configuration the matcher cannot check fails closed for every
	// certificate, even when another candidate would match.
	require.ErrorIs(t, mtls.SubjectValidator(unsupported)(cert), mtls.ErrUnsupportedDNAttribute)
	require.ErrorIs(t, mtls.IssuerValidator(unsupported)(cert), mtls.ErrUnsupportedDNAttribute)
	require.ErrorIs(t, mtls.SubjectValidator(fullDN(), unsupported)(cert), mtls.ErrUnsupportedDNAttribute)
}

func TestDNValidatorsCopyConfiguration(t *testing.T) {
	t.Parallel()
	cert := parsedDNCert(t, fullDN())
	want := pkix.Name{
		Locality:   []string{"Shelbyville"},
		ExtraNames: []pkix.AttributeTypeAndValue{{Type: oidTitle, Value: "admin"}},
	}
	subject := mtls.SubjectValidator(want)

	// Mutating the caller's slices after construction must not change policy.
	want.Locality[0] = "Springfield"
	want.ExtraNames[0].Value = "service"
	require.ErrorIs(t, subject(cert), mtls.ErrSubjectMismatch)
}
