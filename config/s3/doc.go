// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package s3config defines the S3 connection schema shared by TLS certificate
// providers and OPA bundle sources.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-atlas/config/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [Config].
package s3config
