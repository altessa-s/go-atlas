// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import "errors"

// Sentinel errors returned by the plugin manager.
var (
	// ErrPluginNotFound is returned when a plugin is not registered in the manager.
	ErrPluginNotFound = errors.New("plugin not found")

	// ErrPluginAlreadyLoaded is returned when attempting to load a plugin
	// whose name conflicts with an already loaded plugin.
	ErrPluginAlreadyLoaded = errors.New("plugin already loaded")

	// ErrPluginFailed is returned when a plugin's Init function returns an error.
	ErrPluginFailed = errors.New("plugin failed during initialization")

	// ErrPluginPanicked is returned when a plugin's Init function panics.
	ErrPluginPanicked = errors.New("plugin panicked")

	// ErrUnsupportedPlatform is returned on platforms that do not support
	// Go's plugin package (anything other than darwin/linux).
	ErrUnsupportedPlatform = errors.New("plugins not supported on this platform")

	// ErrNoDescriptor is returned when a loaded .so file does not export
	// the required Descriptor symbol.
	ErrNoDescriptor = errors.New("plugin missing Descriptor symbol")

	// ErrInvalidDescriptor is returned when the exported Descriptor symbol
	// is not of type *Descriptor.
	ErrInvalidDescriptor = errors.New("plugin Descriptor has invalid type")

	// ErrManagerClosed is returned when operations are attempted on a closed manager.
	ErrManagerClosed = errors.New("plugin manager is closed")

	// ErrDirNotFound is returned when the configured plugin directory does not exist.
	ErrDirNotFound = errors.New("plugin directory not found")

	// ErrSandboxUnsupported is returned when the plugin sandbox is enabled on
	// a platform that does not support the underlying primitives (anything
	// other than linux).
	ErrSandboxUnsupported = errors.New("plugin sandbox not supported on this platform")

	// ErrSandboxFailed is returned when applying the plugin sandbox primitives
	// (PR_SET_NO_NEW_PRIVS, setrlimit) fails. The wrapped error preserves the
	// underlying syscall error for inspection.
	ErrSandboxFailed = errors.New("plugin sandbox setup failed")
)
