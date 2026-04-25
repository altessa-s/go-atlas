// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package config defines the configuration structures and validation logic for
// all infrastructure components managed by go-atlas.
//
// # Structure Types
//
// Each infrastructure concern is represented by one or more exported structs
// (e.g. [Mongodb], [Nats], [Redis], [Grpc], [Http], [Auth], [Observability]).
// Struct fields carry YAML struct tags and default-value tags; the
// [config/loader] package populates them from files, environment variables,
// and secret references.
//
// Every top-level config struct exposes:
//   - A Default* constructor that returns the struct with sensible defaults.
//   - A Validate method that uses ozzo-validation to enforce invariants.
//   - A Normalize method (where applicable) that initializes sub-structs
//     implied by a type-selector field.
//
// # Storage Pattern
//
// Several features (rate limiting, idempotency, caching) share a common
// storage abstraction: a type-selector field ([CacheStorageType]) paired with
// provider-specific sub-configs ([StorageMemoryConfig], [StorageRedisConfig],
// [StorageNATSConfig]). The [CacheStorageConfig] struct implements this
// pattern; call Normalize before Validate to allocate the right sub-struct.
//
// # Interceptors and Middlewares
//
// gRPC interceptor configs embed [BaseGrpcInterceptorConfig] which bundles
// [EnableMixin] and [InterceptorFilterConfig]. HTTP middleware configs embed
// [BaseHttpMiddlewareConfig] with [HttpMiddlewareFilterConfig]. Both provide
// shared enable/disable and method/path exclusion logic so that concrete
// interceptor and middleware configs only declare their unique fields.
//
// # Secret Handling
//
// The [Secret] type wraps sensitive strings and automatically redacts them in
// fmt, JSON, YAML, and slog output. Use it for passwords, tokens, and keys.
//
// # Subpackages
//
//   - [config/loader]: Multi-source configuration loading (YAML/TOML files,
//     environment variables, default tags, secret expansion).
//   - [config/loader/secrets]: Secret placeholder expansion ($__secret{ns:key}).
//   - [config/loader/backend]: Backend interface and implementations (YAML, TOML).
//   - [config/internal/utils]: Internal file-lookup helpers.
//   - [config/internal/validators]: Internal custom validators.
//   - templates: Default configuration file templates (no Go source).
package config
