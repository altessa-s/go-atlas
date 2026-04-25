// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package gcp provides an implementation of the secrets.Provider interface
// for Google Cloud Platform's Secret Manager service. This provider enables
// secure retrieval and management of secrets stored in GCP Secret Manager
// with support for concurrent operations, retry logic, and base64 encoding.
//
// The GCP provider supports complete CRUD operations, automatic concurrent
// retrieval, and comprehensive error handling with GCP-specific error mapping.
// All operations are performed using the official Google Cloud Secret Manager
// client with proper authentication and connection management.
//
// Authentication is handled through GCP service account credentials, and the
// provider requires appropriate IAM permissions for Secret Manager operations.
//
// Example:
//
//	// Create a new GCP provider with basic configuration
//	provider, err := gcp.New(
//		"my-project-id",
//		"/path/to/service-account.json",
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Create a provider with label filtering
//	provider, err := gcp.New(
//		"my-project-id",
//		"/path/to/service-account.json",
//		gcp.WithLabels(map[string]string{
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
//   - secretmanager.admin or secretmanager.secretAccessor for reading secrets
//   - secretmanager.secretVersionManager for creating/updating secrets
//   - secretmanager.secretDeleter for deleting secrets
//
// Configuration Options:
//
// The provider supports configuration through functional options:
//   - WithLabels: Filter secrets by GCP labels for organization and access control
//
// Thread Safety:
//
// All provider operations are thread-safe and can be used concurrently from
// multiple goroutines. The underlying GCP client handles connection pooling
// and request management automatically.
package gcp
