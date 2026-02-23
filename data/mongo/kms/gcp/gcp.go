// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp

import (
	"crypto/tls"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/data/mongo/kms"
)

// Google represents a Google Cloud KMS provider configuration.
// It implements the kms.Provider, kms.MasterKey, and kms.TLSConfig interfaces.
// Credentials are stored securely using the credentials package.
type Google struct {
	credentialsCache atomic.Pointer[kms.Credentials]
	// Endpoint is the optional custom Google Cloud KMS endpoint URL
	Endpoint *string
	// ProjectID is the Google Cloud project ID
	ProjectID string
	// credentials stores sensitive GCP service account authentication data securely
	credentials *GCPCredentials
	// AuthenticationEndpoint is the optional custom authentication endpoint URL
	AuthenticationEndpoint *string
	// Location is the location of the key ring (e.g., "global", "us-east1")
	Location string
	// KeyRing is the name of the key ring in Google Cloud KMS
	KeyRing string
	// KeyName is the name of the key in the key ring
	KeyName string
	// KeyVersion is the optional version of the key
	KeyVersion *string
	// TLS is the optional custom TLS configuration
	TLS *tls.Config
}

// New creates a new Google Cloud KMS provider with the specified credentials and key.
// Credentials are stored securely and automatically cleared when no longer needed.
//
// Parameters:
//   - projectID: Google Cloud project ID
//   - email: Service account email address (will be stored securely)
//   - privateKey: Service account private key in PEM format (will be stored securely)
//   - location: Location of the key ring (e.g., "global", "us-east1")
//   - keyRing: Name of the key ring in Google Cloud KMS
//   - keyName: Name of the key in the key ring
//   - opts: Optional configuration options
//
// Example:
//
//	gcp := kmsgcp.New("my-project", "service@project.iam.gserviceaccount.com",
//		"<key>", "global", "my-ring", "my-key",
//		kmsgcp.WithKeyVersion("1"),
//		kmsgcp.WithEndpoint("https://cloudkms.googleapis.com"))
func New(projectID, email, privateKey, location, keyRing, keyName string, opts ...Option) *Google {
	options := newOptions(opts...)

	gcp := &Google{
		ProjectID:              projectID,
		credentials:            NewGCPCredentials(email, privateKey),
		Location:               location,
		KeyRing:                keyRing,
		KeyName:                keyName,
		Endpoint:               options.endpoint,
		AuthenticationEndpoint: options.authenticationEndpoint,
		KeyVersion:             options.keyVersion,
		TLS:                    options.tLS,
	}

	return gcp
}

// Name returns the provider name "gcp".
// This implements the kms.Provider interface.
func (g *Google) Name() string {
	return ProviderName
}

// Credentials returns the Google Cloud service account credentials.
// This implements the kms.Provider interface.
// Note: This method exposes sensitive credentials - use with caution.
func (g *Google) Credentials() kms.Credentials {
	if cred := g.credentialsCache.Load(); cred != nil {
		return *cred
	}

	if g.credentials == nil {
		return kms.Credentials{g.Name(): {}}
	}

	cred := kms.Credentials{
		g.Name(): {
			Email:         g.credentials.Email.StringUnsafe(),
			GCPPrivateKey: g.credentials.PrivateKey.StringUnsafe(),
		},
	}

	if g.AuthenticationEndpoint != nil {
		cred[g.Name()][Endpoint] = *g.AuthenticationEndpoint
	}

	g.credentialsCache.Store(&cred)

	return cred
}

// MasterKey returns the Google Cloud KMS master key configuration.
// This implements the kms.MasterKey interface.
func (g *Google) MasterKey() kms.Key {
	key := kms.Key{
		ProjectID:   g.ProjectID,
		GCPLocation: g.Location,
		KeyRing:     g.KeyRing,
		GCPKeyName:  g.KeyName,
	}

	if g.KeyVersion != nil {
		key[KeyVersion] = *g.KeyVersion
	}

	if g.Endpoint != nil {
		key[Endpoint] = *g.Endpoint
	}

	return key
}

// TLSConfig returns the TLS configuration for Google Cloud KMS connections.
// This implements the simplified kms.Provider interface.
func (g *Google) TLSConfig() *tls.Config {
	return g.TLS
}

// Clear securely clears all stored credentials and sensitive data.
// This method should be called when the GCP KMS provider is no longer needed
// to ensure sensitive credentials don't remain in memory.
func (g *Google) Clear() {
	if g.credentials != nil {
		g.credentials.Clear()
		g.credentials = nil
	}
}
