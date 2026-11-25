// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqlident validates table and schema names shared by configuration
// validators and SQL storages.
//
// # Usage
//
//	if err := sqlident.Validate("app.events"); err != nil {
//	    return err
//	}
package sqlident
