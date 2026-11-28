// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go tool optgen generate --type=options

type options struct {
	// configuredNames serves the configured names only: a handshake without a server name, or with a name outside
	// the common and alternative names, gets the certificate of the common name, so the provider never asks Vault for
	// a name the role refuses. Without it every server name is asked of Vault.
	configuredNames bool `opt:"ConfiguredNames"`

	// issueOnBuild makes tlsfactory.ProvidersBuilder.Build obtain the certificate of every configured name before it
	// returns, so a server never starts without one. Build retries until its context ends and fails when Vault issues
	// none, or issues one that does not outlive renewBefore.
	issueOnBuild bool `opt:"IssueOnBuild"`
}
