// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build darwin || linux

package plugins

import (
	"path/filepath"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	goplugin "plugin"
)

// openPlugin opens a .so file via Go's plugin package.
// The path is resolved to absolute before opening.
func openPlugin(path string) (*goplugin.Plugin, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "failed to resolve plugin path %q", path)
	}
	p, err := goplugin.Open(abs)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "failed to open plugin %q", abs)
	}
	return p, nil
}
