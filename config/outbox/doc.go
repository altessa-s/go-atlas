// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outboxconfig defines the transactional outbox schema used by
// standalone outbox factories and by the broker's outbox YAML block.
// [Config.Normalize] supplies missing storage defaults before validation.
// [Config.ValidateStorage] checks the watch mode and its dispatch schedule.
package outboxconfig
