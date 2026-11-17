// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import "context"

// Close permanently stops background purging and waits for it within ctx.
// Explicit PurgeExpired calls remain available. It does not close the database.
func (s *Store[P]) Close(ctx context.Context) error { return s.purge.Close(ctx) }
