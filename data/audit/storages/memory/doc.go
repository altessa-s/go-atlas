// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory [audit.Storage] implementation.
// It is intended for testing and development; all events are stored in
// a slice protected by a sync.RWMutex and lost on process exit.
package memory
