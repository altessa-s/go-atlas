// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package irreversible runs store/postgres/migrations with a migration registered without a down step. It is a test binary of its
// own because the migration registry is process-wide: its migrations must not join those of postgresit.
package irreversible
