// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package validators provides custom validation rules for configuration fields.
// Extends ozzo-validation with domain-specific validators for MongoDB, network addresses,
// file paths, and certificates. Internal package; API may change without notice.
//
// Example:
//
//	validation.Validate(mongoUri, validators.MongoURI())
//	validation.Validate(path, validators.FileExists())
package validators
