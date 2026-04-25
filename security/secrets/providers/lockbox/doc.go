// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package lockbox provides an implementation of the secrets.Provider interface
// for Yandex Cloud Lockbox service. This provider enables secure retrieval
// and management of secrets stored in Yandex Cloud Lockbox with support for
// concurrent operations, IAM authentication, and base64 encoding.
//
// The Lockbox provider supports complete CRUD operations, automatic concurrent
// retrieval, and comprehensive error handling with Yandex Cloud error mapping.
// It uses a dual-client architecture with separate clients for secret management
// and payload retrieval operations, optimizing performance and security.
//
// Authentication is handled through Yandex Cloud service account key authentication
// with automatic JWT token generation and refresh. The provider requires appropriate
// IAM permissions for Lockbox operations.
//
// Example:
//
//	// Create a new Lockbox provider with basic configuration
//	privKey, err := os.ReadFile("/path/to/service-account-key.pem")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	provider, err := lockbox.New(
//		"b1g123example456",  // folder ID
//		"key-id-123",        // key ID
//		"sa-id-456",         // service account ID
//		privKey,             // private key bytes
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Create a provider with label filtering
//	provider, err := lockbox.New(
//		"b1g123example456",
//		"key-id-123",
//		"sa-id-456",
//		privKey,
//		lockbox.WithLabels(map[string]string{
//			"env":     "production",
//			"service": "api",
//		}),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
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
// Required IAM Permissions:
//
// The service account used must have the following IAM roles or equivalent permissions:
//   - lockbox.admin or lockbox.viewer for reading secrets
//   - lockbox.editor for creating/updating secrets
//   - lockbox.admin for deleting secrets
//   - resourcemanager.clouds.member for folder access
//
// Service Account Key Setup:
//
// The provider requires a Yandex Cloud service account with a private key in PEM format.
// To create the required credentials:
//  1. Create a service account in Yandex Cloud Console
//  2. Assign appropriate Lockbox permissions
//  3. Create and download a service account key
//  4. Extract the private key from the JSON file
//
// Configuration Options:
//
// The provider supports configuration through functional options:
//   - WithLabels: Filter secrets by Yandex Cloud labels for organization and access control
//
// Thread Safety:
//
// All provider operations are thread-safe and can be used concurrently from
// multiple goroutines. The provider uses appropriate synchronization mechanisms
// and the underlying gRPC clients handle connection pooling automatically.
package lockbox
