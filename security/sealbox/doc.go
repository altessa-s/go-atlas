// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sealbox seals and opens small secrets at rest (API keys, webhook secrets, client credentials) with AES-256-GCM and a
// keyring of master keys.
//
// # Keys and rotation
//
// [New] takes the current master key and any number of previous ones, each the standard base64 encoding of 32 random bytes (for
// example from configuration or a secret manager). The current key seals; every configured key opens. To rotate, configure a new
// current key and move the old one to the previous keys; [Box.Current] tells which stored values still use an older key so they can be
// re-sealed on their next save, after which the old key can be dropped. An empty current key yields a nil [*Box] whose methods fail
// with [ErrNoKey], so a service can run without secrets storage configured.
//
// # Associated data
//
// Every value is bound to associated data chosen by the caller (for example "provider:<id>"): it opens only with the same bytes, so a
// ciphertext copied onto another record does not decrypt.
//
// # Format
//
// A [Sealed] value holds three fields that are stored as they are:
//
//   - KeyID: the first 16 hex digits of the SHA-256 of the raw key ([KeyID]);
//   - Nonce: 12 random bytes, fresh for every seal;
//   - Ciphertext: the GCM output, the encrypted plaintext followed by the 16-byte tag.
//
// The format is stable: values sealed by earlier versions keep opening.
//
// # Usage
//
//	box, err := sealbox.New(cfg.MasterKey, cfg.PreviousMasterKeys...)
//	if err != nil {
//	    return err
//	}
//	sealed, err := box.Seal([]byte(apiKey), []byte("provider:"+id))
//	// store sealed.KeyID, sealed.Nonce, sealed.Ciphertext
//	plain, err := box.Open(sealed, []byte("provider:"+id))
package sealbox
