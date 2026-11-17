// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package validators provides custom validation rules for configuration fields.
// SQLTableName and SQLIdentifier validate SQL names through the shared dialect
// parser. MongoDirectionConnect checks MongoDB connection settings.
// Internal package; API may change without notice.
//
// # Usage
//
//	validation.Validate(table, validators.SQLTableName)
//	validation.Validate(channel, validators.SQLIdentifier)
package validators
