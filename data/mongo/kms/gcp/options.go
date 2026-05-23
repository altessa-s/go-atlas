// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"crypto/tls"
)

// options contains Google Cloud KMS provider configuration.
type options struct {
	// Endpoint sets a custom Google Cloud KMS endpoint URL.
	// This is typically used for testing or when using Google Cloud-compatible services.
	// Example: "https://cloudkms.googleapis.com" or custom endpoint for testing
	endpoint *string

	// AuthenticationEndpoint sets a custom authentication endpoint URL.
	// This is typically used for testing or when using custom authentication servers.
	// Example: "https://oauth2.googleapis.com/token" or custom endpoint for testing
	authenticationEndpoint *string

	// KeyVersion sets the version of the key in Google Cloud KMS.
	// This is optional and allows using a specific version of the key.
	// If not specified, the latest version will be used.
	keyVersion *string

	// TLS sets a custom TLS configuration for connections to Google Cloud KMS.
	// This allows fine-grained control over TLS settings for secure connections.
	tLS *tls.Config
}
