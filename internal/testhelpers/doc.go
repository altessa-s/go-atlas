// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package testhelpers provides common testing utilities and helpers
// shared across the go-atlas project.
//
// The package includes:
//   - A network error double ([MockNetError]).
//   - HTTP and I/O test doubles ([RoundTripFunc], [MockReadCloser]).
//   - TLS and cryptographic utilities: an in-memory ECDSA test CA ([NewCA],
//     [CA.SignLeaf] with [CertOption] values for SANs, EKUs, serials, and
//     OCSP URLs), self-signed certificates ([SelfSignedCert]), key generation
//     ([GenerateRSAKey], [GenerateECDSAKey], [GenerateEd25519Key]), and
//     PEM-file plumbing ([WriteTempCertFiles]).
//   - A scheduler double ([MockTaskRegistrar]) that records registered
//     [core/scheduler.TaskConfig] values instead of running them, so a test can
//     assert the task ID and schedule a component derived from configuration.
//   - A scripted database/sql driver ([NewFakeSQL], [FakeSQL]) that records
//     every statement, its arguments and whether it ran in a transaction, so
//     SQL-building code is testable without a server or a third-party driver.
//   - A protobuf update helper ([ApplyLeafPaths]) that copies the fields an
//     update mask names leaf by leaf, as a storage update by leaf paths
//     would, to check which stored values an update leaves intact.
//   - Polling helpers ([WaitFor]).
//
// The package depends only on core and the standard library. Fixtures tied to
// a capability live with it: audit/audittest,
// reliability/idempotency/idempotencytest and
// observability/metrics/metricstest; in-process NATS and Redis servers live in
// internal/testhelpers/servers/{natstest,redistest}.
//   - The run length of the `stress`-tagged soak tests ([StressDuration],
//     set through [StressDurationEnv]).
//
// All helpers that accept [testing.TB] register cleanup functions automatically,
// so callers do not need to manage teardown manually.
package testhelpers
