// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package testhelpers provides common testing utilities and helpers
// shared across the go-atlas project.
//
// The package includes:
//   - Mock providers for idempotency ([MockIdempotencyStorage]) and
//     network errors ([MockNetError]).
//   - In-memory metrics test helpers ([NewTestCollector], [GetCounterValue],
//     [GetGaugeValue], [GetHistogramCount], [GatherMetric]). The collector
//     stores observations in memory, so tests do not depend on Prometheus.
//   - Filter expression parsing ([MustParseFilter]) and order_by parsing
//     ([MustParseOrderBy]).
//   - Pointer helpers ([StringPtr], [IntPtr], [TimePtr]).
//   - HTTP and I/O test doubles ([RoundTripFunc], [MockReadCloser]).
//   - NATS/JetStream helpers for spinning up embedded servers ([StartNATSServer],
//     [ConnectNATS], [ConnectJetStream], [CreateNATSKV]) and capturing bucket
//     configuration ([JetStreamKVCapture]).
//   - Redis bootstrap ([RedisClient]) that starts an in-process miniredis
//     server and returns a connected go-redis client plus the server handle.
//   - TLS and cryptographic utilities: an in-memory ECDSA test CA ([NewCA],
//     [CA.SignLeaf] with [CertOption] values for SANs, EKUs, serials, and
//     OCSP URLs), self-signed certificates ([SelfSignedCert]), key generation
//     ([GenerateRSAKey], [GenerateECDSAKey], [GenerateEd25519Key]), and
//     PEM-file plumbing ([WriteTempCertFiles]).
//   - A scheduler double ([MockTaskRegistrar]) that records registered
//     [core/scheduler.TaskConfig] values instead of running them, so a test can
//     assert the task ID and schedule a component derived from configuration.
//   - Polling helpers ([WaitFor]) and an audit helper ([NewTestAuditor]).
//
// All helpers that accept [testing.TB] register cleanup functions automatically,
// so callers do not need to manage teardown manually.
package testhelpers
