// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds security/hmacsign signers and verifiers from a
// [github.com/altessa-s/go-atlas/config.WebhookSignature] template.
//
// [Verifier] authenticates inbound webhooks; [Signer] signs outbound ones. Both
// resolve the provider scheme from the config string, expose the secret through
// [config.Secret], and carry additional rotation secrets and the replay
// tolerance into the built object.
//
//	v, err := factory.Verifier(cfg.WebhookSignature)
//	if err != nil {
//	    return err
//	}
//	mux.Handle("/webhooks", httpsign.Middleware(v)(handler))
package factory
