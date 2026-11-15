// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds a [github.com/altessa-s/go-aip/pagination/pagetoken.Codec]
// from the shared paging schema
// [github.com/altessa-s/go-atlas/config/paging.Config]: it decodes the signing
// and previous keys under the configured key encoding and maps the token
// lifetime and the encryption block (pagetoken.WithEncryption,
// pagetoken.WithStrictEncryption). Every config-driven page-token codec (audit queries, scheduler
// lists, service List RPCs) is built here.
//
// # Usage
//
//	codec, err := factory.New(cfg.Paging).Build()
//	if err != nil {
//	    return err
//	}
//	tokens := audit.NewPageTokens(codec)
package factory
