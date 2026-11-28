// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postgresconfig defines the PostgreSQL pool and timeout
// schema. [Default] supplies defaults for programmatic configuration. The YAML
// and default tags support the go-nomos loader. [Config.Validate] checks the
// configuration before the factory opens any connections. [Migrations]
// configures a direct migration session that [Config.MigrationSession]
// derives from the primary settings.
package postgresconfig
