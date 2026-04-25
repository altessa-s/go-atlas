// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an implementation of the secrets.Provider interface
// for in-memory secret storage. This provider is useful for testing,
// development environments, or when working with a fixed set of secrets that
// don't change during application runtime.
//
// The memory provider loads all secrets from a provided map during initialization
// and serves them from memory. It implements the Static interface, indicating
// that secrets don't change over time, which allows the manager to skip
// periodic refresh operations for optimal performance.
//
// This provider is particularly useful in scenarios where:
//   - Testing applications with known secret values
//   - Development environments with fixed configuration
//   - Applications with embedded secrets that don't change
//   - Performance-critical applications requiring zero-latency secret access
//   - Offline environments without access to external secret services
//
// Example:
//
//	// Create a memory provider with a set of secrets
//	secrets := map[string]string{
//		"database_url":    "postgres://localhost:5432/mydb",
//		"redis_url":       "redis://localhost:6379",
//		"api_key":         "dev-api-key-12345",
//		"jwt_secret":      "jwt-signing-secret",
//		"encryption_key":  "32-byte-encryption-key-here!!!",
//	}
//	provider := memory.New(secrets)
//
//	// Use with secrets manager
//	manager := secrets.New(provider)
//	// Note: RunUpdateCycle() is not needed for static providers
//
//	// Retrieve secrets (very fast, no network calls)
//	dbUrl, err := manager.Value(ctx, "database_url", true)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Database URL: %s\n", dbUrl.Value)
//
//	// List all available secrets
//	allSecrets, err := provider.List(context.Background())
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Loaded %d secrets\n", len(allSecrets))
//
// Testing Example:
//
//	// Perfect for unit tests with known secret values
//	func TestSecretRetrieval(t *testing.T) {
//		testSecrets := map[string]string{
//			"test_key": "test_value",
//		}
//		provider := memory.New(testSecrets)
//		manager := secrets.New(provider)
//
//		value, err := manager.Value(ctx, "test_key", true)
//		assert.NoError(t, err)
//		assert.Equal(t, "test_value", value.Value)
//	}
//
// Thread Safety:
//
// All provider operations are thread-safe and can be used concurrently from
// multiple goroutines. The provider uses read-write mutex protection to ensure
// data consistency while allowing concurrent read operations for optimal performance.
//
// Limitations:
//
// Consider these limitations when using the memory provider:
//   - Secrets are stored in plain text in application memory
//   - Not suitable for production environments with sensitive secrets
//   - No persistence across application restarts
//   - Memory usage scales linearly with the number and size of secrets
//   - No encryption or additional security measures
package memory
