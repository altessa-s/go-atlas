// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !darwin && !linux

package commands

import "fmt"

// loadExternalPlugins is the non-unix stub that rejects plugin loading
// on unsupported platforms. It returns nil when paths is empty.
func loadExternalPlugins(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return fmt.Errorf("external plugins are not supported on this OS/arch (use darwin/linux)")
}
