// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory creates [kms.Provider] instances from
// [config.MongoKMS] configuration, routing to the appropriate cloud
// or local KMS implementation (AWS, Azure, GCP, local).
package factory
