// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package explain executes MongoDB aggregation explain commands and reports query statistics.
//
// # Usage
//
//	stats, err := explain.Run(ctx, collection, pipeline, nil, logger)
//	if err != nil {
//		return err
//	}
//	_ = stats.IndexUsed
package explain
