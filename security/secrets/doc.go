// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package secrets provides centralized secret management with automatic caching,
// typed errors, and real-time watch capabilities.
// It is the primary interface for applications to access secrets from various
// storage providers; an LRU cache keeps frequently used secrets in memory.
//
// The package supports multiple storage backends including Google Cloud Secret Manager,
// HashiCorp Vault, Yandex Cloud Lockbox, and in-memory storage. All operations are
// thread-safe and designed for concurrent access from multiple goroutines.
//
// Key Features:
//   - Unified Provider[T] interface for multiple secret storage backends
//   - Automatic LRU caching with configurable size limits and eviction policies
//   - Flexible access patterns: cache-only (force=false) or cache-with-fallback (force=true)
//   - Full CRUD operations: Create, Read, Update, Delete secrets with version tracking
//   - Real-time Watch API for secret change notifications with event filtering
//   - Scheduler-based cache updates via RunUpdateCycle for periodic synchronization
//   - Concurrent secret retrieval with worker pools
//   - Generic type support for different secret value types (string, []byte, custom structs)
//   - Thread-safe operations with proper synchronization and atomic operations
//   - Configurable encoding/decoding for keys and values with codec interface
//   - Graceful error handling with provider-specific error mapping and standard error types
//   - Health monitoring and connection validation for storage providers
//   - Graceful shutdown with configurable timeout and secure memory clearing
//
// Architecture Overview:
//
// The library is built around four core components:
//   - Provider[T]: Interface for secret storage backends with CRUD operations
//   - Manager[T]: Central orchestrator for caching, updates, and lifecycle management
//   - Value[T]: Container for secret data with metadata and security features
//   - WatchManager[T]: Real-time event system for secret change notifications
//
// The Manager[T] acts as the primary interface for applications, handling LRU caching,
// background updates, and provider abstraction. Providers implement the storage
// logic for specific backends like HashiCorp Vault, Google Cloud Secret Manager,
// or Yandex Cloud Lockbox. The WatchManager provides real-time notifications when
// secrets are created, updated, or deleted, so applications can react immediately
// to configuration changes.
//
// Basic Usage Example:
//
//	package main
//
//	import (
//		"context"
//		"errors"
//		"fmt"
//		"log"
//		"time"
//
//		"github.com/altessa-s/go-atlas/security/secrets"
//		"github.com/altessa-s/go-atlas/security/secrets/providers/vault"
//		corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
//		"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
//	)
//
//	func main() {
//		ctx := context.Background()
//
//		// Create a Vault provider
//		client, err := vault.NewClient(vault.DefaultConfig())
//		if err != nil {
//			log.Fatal(err)
//		}
//		client.SetToken(os.Getenv("VAULT_TOKEN"))
//
//		provider := vault.New(client,
//			vault.WithMountPath("kv"),
//			vault.WithSecretPath("myapp"),
//		)
//
//		// Create manager with configuration
//		manager := secrets.New(provider)
//		defer manager.Shutdown()
//
//		// Register periodic cache updates with scheduler
//		sched := scheduler.New(memory.New(100))
//		sched.Register(ctx, corescheduler.TaskConfig{
//			ID:         "secrets-cache-sync",
//			Interval:   5 * time.Minute,
//			Func:       manager.RunUpdateCycle,
//			RunOnStart: true, // Perform initial sync immediately
//		})
//		sched.Start(ctx)
//		defer sched.Stop(ctx)
//
//		// Retrieve a secret with cache-first pattern
//		secret, err := manager.Value(ctx, "database_password", false)
//		if errors.Is(err, secrets.ErrNotFound) {
//			// Not in cache, fetch from storage
//			secret, err = manager.Value(ctx, "database_password", true)
//		}
//		if err != nil {
//			log.Fatalf("Failed to retrieve secret: %v", err)
//		}
//		defer secret.Clear()
//
//		fmt.Printf("Secret retrieved (version: %s)\n", secret.Version)
//	}
//
// Supported Providers:
//
// The package includes built-in support for several secret storage providers:
//
//   - providers/gcp: Google Cloud Secret Manager with automatic authentication
//   - providers/vault: HashiCorp Vault KV v2 engine with token management
//   - providers/lockbox: Yandex Cloud Lockbox with service account authentication
//   - providers/memory: In-memory storage for testing and development
//
// Each provider implements the Provider[T] interface with full CRUD operations,
// automatic retry logic, and provider-specific optimizations for performance
// and reliability.
//
// # Provider Interface:
//
// All providers implement the Provider[T] interface:
//
//	type Provider[T any] interface {
//		Name() string
//		List(ctx context.Context) ([]*Value[T], error)
//		Value(ctx context.Context, key string) (*Value[T], error)
//		Save(ctx context.Context, key string, value T) error
//		Delete(ctx context.Context, key string) error
//	}
//
// Static providers can additionally implement the Static interface to indicate
// that their secrets don't change over time, which lets the Manager skip periodic refresh.
//
// # Caching Strategy:
//
// The Manager uses an LRU cache to store frequently accessed secrets in memory:
//   - Configurable maximum size with automatic eviction
//   - Thread-safe concurrent access with read-write locks
//   - Flexible access patterns: cache-only (force=false) or cache-with-fallback (force=true)
//   - Cache warming for critical secrets at startup
//   - Background updates for dynamic providers
//   - Manual cache invalidation and updates
//   - Automatic cache population on successful Save operations
//
// # Periodic Cache Updates:
//
// For dynamic providers, use RunUpdateCycle with an external scheduler:
//   - Register RunUpdateCycle as a task in service/scheduler for periodic sync
//   - Retry logic with exponential backoff for failed requests
//   - Graceful handling of temporary network issues
//   - Atomic cache updates to maintain consistency
//   - Full control over update scheduling and lifecycle
//
// # Error Handling:
//
// Error handling includes:
//   - Standard error types (ErrNotFound, ErrDecoding, etc.)
//   - Provider-specific error mapping
//   - Retry logic for transient failures
//   - Graceful degradation during outages
//   - Detailed error context and logging
//
// # Security:
//
// Secret material never reaches the log. Debug-level records from the Manager
// carry the secret's key — the lookup identifier validated against
// [a-zA-Z0-9_.-]+, not the value behind it — so cache miss, fetch, save and
// delete records for one secret can be correlated.
//
// The payload itself is protected on two levels. Value.Value and
// Value.EncodedValue carry json:"-", so an accidental json.Marshal of a
// [Value] emits metadata only. [Value.LogValue] implements [slog.LogValuer]
// and reports the same metadata, so a Value passed to a logger is redacted;
// slog resolves a LogValuer atomically, which means the secret fields cannot
// leak through field expansion either.
//
// Key names are internal identifiers, but a naming scheme can itself disclose
// infrastructure layout. When that matters, run production loggers at Info or
// above, or wrap the handler with the masking handler from
// [github.com/altessa-s/go-atlas/observability/slog/handler/masking].
//
// # Thread Safety:
//
// All operations are thread-safe and designed for concurrent use:
//   - Atomic operations for state management
//   - Mutex protection for shared data structures
//   - Lock-free cache operations where possible
//   - Proper goroutine lifecycle management
//   - Context support for cancellation and timeouts
//
// # Performance Characteristics:
//
// The library is optimized for high-performance secret retrieval:
//   - O(1) cache lookups for frequently accessed secrets
//   - Concurrent provider operations with worker pools
//   - Minimal memory overhead with LRU eviction
//   - Background updates don't block foreground operations
//   - Efficient encoding/decoding with reusable components
//   - Connection pooling and keepalive for network providers
//
// # Testing and Development:
//
// The memory provider is perfect for testing and development:
//   - No external dependencies or network calls
//   - Predictable behavior with known secret values
//   - Immediate availability upon initialization
//   - Full CRUD support for dynamic testing scenarios
//   - Zero latency for performance-sensitive tests
package secrets
