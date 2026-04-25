// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp

// ProviderName is the identifier for Google Cloud KMS provider
const ProviderName = "gcp"

const (
	// Email is the credential field name for GCP service account email
	Email = "email"

	// GCPPrivateKey is the credential field name for GCP service account private key
	GCPPrivateKey = "privateKey"

	Endpoint = "endpoint"
)

const (
	// ProjectID is the master key field name for GCP project ID
	ProjectID = "projectId"

	// GCPLocation is the master key field name for GCP location
	GCPLocation = "location"

	// KeyRing is the master key field name for GCP key ring
	KeyRing = "keyRing"

	// GCPKeyName is the master key field name for GCP key name
	GCPKeyName = "keyName"

	// KeyVersion is the master key field name for GCP key version (optional)
	KeyVersion = "keyVersion"
)
