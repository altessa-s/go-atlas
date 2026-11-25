// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package validators provides custom validation rules for configuration fields.
// SQLTableName and SQLIdentifier validate SQL names with internal/sqlident. MongoDirectionConnect checks MongoDB connection settings.
// RetryQueueFlagField checks a flag field against the MongoDB retry queue schema.
// CIDRPrefix and HTTPSURL validate network prefixes and endpoint URLs.
// [BackendTypes] admits application-registered storage and provider names.
// Internal package; API may change without notice.
//
// # Usage
//
//	validation.Validate(table, validators.SQLTableName)
//	validation.Validate(channel, validators.SQLIdentifier)
package validators
