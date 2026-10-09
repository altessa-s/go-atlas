// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=memoryOptions --output=memory_options_gen.go --option-type=MemoryOption

// MemorySource returns remaining bytes in an explicit memory budget. known is
// false when no budget is available. Sources must be safe for concurrent calls.
// An application can supply OS or container headroom without adding platform
// dependencies to this package.
type MemorySource func() (availableBytes uint64, known bool)

type memoryOptions struct {
	availableMemory MemorySource `optgen:"default=runtimeMemoryHeadroom,notnil"`
}
