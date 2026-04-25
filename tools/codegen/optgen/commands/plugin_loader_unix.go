// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build darwin || linux

package commands

import (
	"path/filepath"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	goplugin "plugin"
)

// loadExternalPlugins opens each .so file via Go's plugin package. The plugin's
// init function is expected to call [plugin.Register] to register its extensions.
// Paths are resolved to absolute before opening. Empty path strings are silently skipped.
func loadExternalPlugins(paths []string) error {
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return coreerrs.Wrapf(err, "failed to resolve plugin path %q", p)
		}
		if _, err := goplugin.Open(abs); err != nil {
			return coreerrs.Wrapf(err, "failed to load plugin %q", abs)
		}
	}
	return nil
}
