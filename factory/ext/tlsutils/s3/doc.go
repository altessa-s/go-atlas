// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the s3 section of the TLS provider configuration to an
// S3 certificate provider for factory/ext/tlsutils, building the S3
// client from the section when none is given.
//
// The TLS factory builds only the file and Let's Encrypt providers; register
// this one for the s3 provider type:
//
//	providers, err := tlsfactory.New(cfg.TLSProvider).
//		UseProvider(tlsproviders.ProviderTypeS3, s3factory.Builder(nil)).
//		Build()
package factory
