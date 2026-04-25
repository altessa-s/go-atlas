// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package testhelpers provides common testing utilities and helpers
// shared across the go-atlas project.
//
// The package includes:
//   - Mock providers for uniqueness ([MockUniqProvider]),
//     idempotency ([MockIdempotencyStorage]), and network errors ([MockNetError]).
//   - Prometheus metrics test helpers ([NewTestCollector], [GetCounterValue],
//     [GetGaugeValue], [GetHistogramCount], [GatherMetric]).
//   - Filter expression parsing ([MustParseFilter]).
//   - Pointer helpers ([StringPtr], [IntPtr], [TimePtr]).
//   - HTTP and I/O test doubles ([RoundTripFunc], [MockReadCloser]).
//   - NATS/JetStream helpers for spinning up embedded servers ([StartNATSServer],
//     [ConnectNATS], [ConnectJetStream], [CreateNATSKV]).
//   - TLS and cryptographic utilities ([SelfSignedCert], [GenerateRSAKey],
//     [WriteTempCertFiles]).
//   - Polling helpers ([WaitFor]) and an audit helper ([NewTestAuditor]).
//
// All helpers that accept [testing.TB] register cleanup functions automatically,
// so callers do not need to manage teardown manually.
package testhelpers
