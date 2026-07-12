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

	// ErrInvalidInit is returned when the optional Init symbol is present
	// but has an unsupported type. The expected signature is
	// func(context.Context) error (or a variable bound to that type).
	ErrInvalidInit = errors.New("plugin Init has invalid type")

	// ErrInvalidDepInfo is returned when the optional DepInfo symbol is
	// present but has an unsupported type. The expected type is
	// *DepInfo (or a variable bound to that type).
	ErrInvalidDepInfo = errors.New("plugin DepInfo has invalid type")

	// ErrSPIVersionMismatch indicates that a plugin's SPI contract version
	// does not satisfy the host's [SPIConstraint]. Consumers of
	// [NegotiateAll] can wrap this sentinel when they want to surface
	// incompatibility as a structured error rather than silently skipping.
	ErrSPIVersionMismatch = errors.New("SPI version mismatch")

	// ErrSignatureInvalid is returned when a plugin's detached .sig file
	// does not verify against the configured public key.
	ErrSignatureInvalid = errors.New("plugin signature invalid")

	// ErrSignatureMissing is returned when signature verification is in
	// "require" mode and the .so.sig companion file does not exist.
	ErrSignatureMissing = errors.New("plugin signature file missing")

	// ErrSignatureConfig is returned when the signature verification
	// configuration is invalid (no public key, unsupported key type,
	// bad PEM file).
	ErrSignatureConfig = errors.New("plugin signature configuration error")

	// ErrPluginQuarantined is returned when a plugin file is blacklisted
	// because a previous load attempt with the same file hash failed.
	// The quarantine is cleared automatically when the file changes
	// (different SHA256). See [Manager.Quarantine] and [Manager.Quarantined].
	ErrPluginQuarantined = errors.New("plugin quarantined")

	// ErrPluginModified is returned when the plugin file's hash after
	// [plugin.Open] differs from the hash that passed signature
	// verification, i.e. the file was swapped on disk inside the
	// verify→open window (TOCTOU). The plugin is quarantined under the
	// new hash and never registered. Note that the swapped file's init
	// code has already run inside plugin.Open — Go cannot unload a
	// plugin — so filesystem permissions remain the primary control; see
	// the Security section in the package documentation.
	ErrPluginModified = errors.New("plugin file changed between signature verification and load")

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

	// ErrHostVersionMismatch is returned when a plugin's [Descriptor.HostVersion]
	// declares a major version that does not match the host service major
	// configured via [WithHostVersion]. The plugin is quarantined and excluded
	// from the registry. Only returned in [HostVersionEnforce] mode; [HostVersionWarn]
	// logs and loads anyway, [HostVersionDisabled] skips the check entirely.
	ErrHostVersionMismatch = errors.New("plugin host major version mismatch")
)
