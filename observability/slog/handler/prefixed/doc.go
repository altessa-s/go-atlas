// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package prefixed provides a slog.Handler middleware that adds prefixes to log messages.
// Use it to categorize logs by component or subsystem.
//
// String attributes under the key set with [WithPrefix] are removed from the
// output and replaced by one attribute under the same key whose value is built
// by the formatter: [DefaultFormatter] renders "[api:server]", [JsonFormatter]
// renders "api:server". Values attached with Logger.With come first, then the
// record's own values. Non-string attributes under the key are left untouched.
//
// Example:
//
//	handler := prefixed.NewHandler(slog.NewTextHandler(os.Stdout, nil),
//		prefixed.WithPrefix("module"))
//	logger := slog.New(handler).With("module", "api")
//	logger.Info("started", "module", "server") // msg=started module=[api:server]
//
// # Groups
//
// A prefix stays at the group level it was attached to. Prefixes collected at
// one level are merged into one attribute; when a group is opened they are
// written at the current level, before the group, and the new group starts
// with no prefixes:
//
//	logger.With("module", "api").WithGroup("req").Info("m", "id", 1)
//	// msg=m module=[api] req.id=1
//
// Within a level the prefix attribute follows that level's other attributes.
package prefixed
