// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds named PostgreSQL pools from configuration.
// [Builder.Build] verifies connectivity, applies enabled migrations and registers
// health checks. The application owns the returned pools and closes them at shutdown.
package factory
