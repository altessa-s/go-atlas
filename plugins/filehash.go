// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"fmt"
	"io"
	"os"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

// maxPluginFileBytes caps how large a .so file the manager is willing to
// read into memory. Real plugins are tens of MB; the cap guards against a
// misplaced huge file in the plugin directory exhausting memory at load time.
const maxPluginFileBytes = 512 << 20 // 512 MiB

// readAndHashFile reads a plugin file and computes the SHA-256 hash of the
// bytes it returns. The digest is always derived from those bytes — never
// cached by path metadata — because signature verification and the
// post-open swap check depend on it describing exactly what was read.
func readAndHashFile(path string) ([]byte, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open plugin: %w", err)
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("stat plugin: %w", err)
	}
	if stat.Size() > maxPluginFileBytes {
		return nil, "", fmt.Errorf("plugin file too large: %d bytes (max %d)", stat.Size(), maxPluginFileBytes)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxPluginFileBytes))
	if err != nil {
		return nil, "", fmt.Errorf("read plugin: %w", err)
	}

	return data, hash.SHA256HexBytes(data), nil
}
