// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !darwin && !linux

package plugins

import goplugin "plugin"

// openPlugin is the stub for unsupported platforms.
// It always returns ErrUnsupportedPlatform.
func openPlugin(_ string) (*goplugin.Plugin, error) {
	return nil, ErrUnsupportedPlatform
}
