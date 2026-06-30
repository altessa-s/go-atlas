// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory turns a [github.com/altessa-s/go-atlas/config.MTLS] into the
// [github.com/altessa-s/go-atlas/auth/mtls] validator options it describes,
// giving the mTLS subsystem the same config→component path the OPA and scope
// factories provide.
//
// Only the certificate-validation policy is declarative — the expiry re-check
// and the trust-domain pin. The identity function, audit recorder, and transport
// label stay at the call site, so [Builder.Options] returns options a transport
// adapter combines with its own.
//
// # Usage
//
//	opts, err := factory.New(&cfg.MTLS).Options()
//	if err != nil {
//	    return err
//	}
//	authFn := grpcmtls.AuthFunc(append(opts, coremtls.WithAudit(rec, subjectOf))...)
package factory
