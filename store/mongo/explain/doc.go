// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package explain executes MongoDB aggregation explain commands and reports query statistics.
// By default [Run] uses [VerbosityQueryPlanner] and omits filter values and raw explain
// payloads from logs. [WithVerbosity] selects the execution mode. [Run] rejects
// unknown modes during command construction. [WithInsecureLogRawExplain] logs the
// explain payload with the contents of $literal expressions redacted.
//
// # Usage
//
//	stats, err := explain.Run(ctx, collection, pipeline, nil, logger)
//	if err != nil {
//		return err
//	}
//	_ = stats.IndexUsed
package explain
