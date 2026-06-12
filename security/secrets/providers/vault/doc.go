// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package vault provides an implementation of the secrets.Provider interface
// for HashiCorp Vault's Key-Value v2 engine. This provider enables secure
// retrieval and management of secrets stored in Vault with support for
// concurrent operations, version management, and flexible path configuration.
//
// The Vault provider supports complete CRUD operations, automatic concurrent
// retrieval, and Vault-specific error mapping.
// It uses Vault's KV v2 engine features: versioning, metadata
// management, and encryption at rest.
//
// Authentication is handled through the provided Vault client, which must be
// properly configured with a valid token and appropriate policies. The provider
// requires specific capabilities for the target paths in Vault.
//
// Example:
//
//	// Create a Vault client with authentication
//	config := vault.DefaultConfig()
//	config.Address = "https://vault.example.com"
//	client, err := vault.NewClient(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Authenticate using token
//	client.SetToken(appinfo.Env(appinfo.EnvVaultToken))
//
//	// Create provider with default settings
//	provider := vault.New(client)
//
//	// Create provider with custom configuration
//	provider := vault.New(
//		client,
//		vault.WithMountPath("kv"),
//		vault.WithSecretPath("myapp/secrets"),
//		vault.WithCAS(), // Enable Check-and-Set operations
//	)
//
//	// Use with secrets manager
//	manager := secrets.New(provider)
//
//	// Register periodic cache updates with scheduler
//	sched.Register(ctx, scheduler.TaskConfig{
//		ID:       "secrets-cache-sync",
//		Interval: 5 * time.Minute,
//		Func:     manager.RunUpdateCycle,
//	})
//
//	// Retrieve a secret
//	value, err := manager.Value(ctx, "database_password", true)
//	if err != nil {
//		log.Printf("Failed to get secret: %v", err)
//		return
//	}
//	fmt.Printf("Secret version: %s\n", value.Version)
//
// Required Vault Capabilities:
//
// The Vault token must have appropriate policies configured for the target paths.
// Required capabilities include:
//   - read: for secret retrieval and metadata access
//   - list: for secret enumeration under configured paths
//   - create: for creating new secrets
//   - update: for updating existing secrets
//   - delete: for secret removal operations
//
// Example Vault policy:
//
//	path "kv/data/myapp/secrets/*" {
//		capabilities = ["read", "create", "update", "delete"]
//	}
//	path "kv/metadata/myapp/secrets/*" {
//		capabilities = ["read", "list", "delete"]
//	}
//
// Path Configuration:
//
// The provider uses a hierarchical path structure in Vault:
//   - Mount Path: Where the KV v2 engine is mounted (default: "kv")
//   - Secret Path: Base path for secrets within the engine (default: "blitz")
//   - Secret Key: Individual secret identifier (encoded with base64)
//
// Full path construction: {mountPath}/data/{secretPath}/{encodedKey}
//
// Check-and-Set Operations:
//
// When CAS is enabled, the provider can perform atomic updates:
//   - Prevents lost updates in concurrent environments
//   - Checks secret version before applying updates
//   - Fails safely if secret was modified by another process
//
// Configuration Options:
//
// The provider supports configuration through functional options:
//   - WithMountPath: Configure the KV v2 engine mount path
//   - WithSecretPath: Set the base path for secrets
//   - WithCAS: Enable Check-and-Set operations for atomic updates
//
// Thread Safety:
//
// All provider operations are thread-safe and can be used concurrently from
// multiple goroutines. The provider uses appropriate synchronization mechanisms
// and the underlying Vault client handles connection pooling automatically.
package vault
