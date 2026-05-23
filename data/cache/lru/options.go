// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

type options struct {
	shardCount int
}
