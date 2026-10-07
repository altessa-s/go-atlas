// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package config is the root of the go-atlas configuration schemas. It
// declares no types: every capability has its own schema package under
// config/, named with a config suffix (grpcconfig, redisconfig, authconfig)
// so it does not clash with driver packages.
//
// Schemas carry yaml and default tags read by config/loader, expose Default*
// constructors and Validate methods, and are mapped to generated options by
// the component factories. Runtime packages never import schemas.
//
// # Schema packages
//
//   - config/validation (validationconfig): shared validation helpers for
//     configuration schemas: struct validation with readable errors and
//     storage-selector checks.
//   - config/storage (storageconfig): shared storage-backend schemas: cache
//     storage selection (memory, NATS, Redis) and NATS JetStream KeyValue
//     settings.
//   - config/middleware (middlewareconfig): schema pieces shared by gRPC
//     interceptors and HTTP middlewares: enable toggles, fallback behavior and
//     IP/geo ACL rules.
//   - config/s3 (s3config): the S3 connection schema shared by TLS certificate
//     providers and OPA bundle sources.
//   - config/proxy (proxyconfig): the outbound HTTP proxy schema used by OIDC,
//     OPA and tracing exporters.
//   - config/retry (retryconfig): the retry policy schema used by outbound
//     clients.
//   - config/tls (tlsconfig): TLS client and server schemas and the
//     certificate provider schemas (file, Let's Encrypt, OCSP, S3, Vault).
//   - config/clienthealth (clienthealthconfig): the client health-check schema
//     for outbound gRPC and HTTP clients.
//   - config/redis (redisconfig): the Redis connection schema.
//   - config/nats (natsconfig): the NATS connection, JetStream consumer and
//     recovery schemas.
//   - config/clickhouse (clickhouseconfig): the ClickHouse connection
//     schema.
//   - config/mongo (mongoconfig): the MongoDB connection, credential and
//     client-side field level encryption schemas.
//   - config/meilisearch (meilisearchconfig): the Meilisearch connection
//     schema.
//   - config/vault (vaultconfig): the HashiCorp Vault client schema.
//   - config/secrets (secretsconfig): the secret manager schema.
//   - config/observability (observabilityconfig): logging, metrics, tracing
//     and health schemas and the aggregate Observability block.
//   - config/probfilter (probfilterconfig): the probabilistic filter (Bloom,
//     Cuckoo) schemas.
//   - config/auth (authconfig): authentication and authorization schemas:
//     OIDC, OPA, mTLS, scopes, denylist, OAuth2 client, SPIFFE and the
//     aggregate Auth block.
//   - config/grpc (grpcconfig): the gRPC server schema and the schemas of its
//     interceptors.
//   - config/http (httpconfig): the HTTP server schema, its middlewares,
//     pprof, TLS and outbound SSRF protection.
//   - config/broker (brokerconfig): the message broker schema with its outbox
//     and in-progress tracking blocks.
//   - config/idempotency (idempotencyconfig): the idempotency key storage
//     schema.
//   - config/limiter (limiterconfig): rate limiter schemas: token bucket,
//     request budget and request limiter.
//   - config/lock (lockconfig): distributed lock and leader election schemas.
//   - config/saga (sagaconfig): the saga orchestration schema.
//   - config/scheduler (schedulerconfig): the distributed scheduler schema.
//   - config/dispatch (dispatchconfig): the async dispatch engine schema and
//     its write-ahead log.
//   - config/audit (auditconfig): the audit event dispatcher schema.
//   - config/plugins (pluginsconfig): the plugin manager and plugin sandbox
//     schemas.
//   - config/webhook (webhookconfig): the webhook signing schema.
//   - config/node (nodeconfig): the service node identity schema.
//
// # Other subpackages
//
//   - config/loader: multi-source configuration loading.
//   - config/templates: commented YAML templates for every schema.
package config
