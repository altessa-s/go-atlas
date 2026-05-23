// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// options holds the configuration for the GitLab policy source.
type options struct {
	// endpoint is the GitLab instance URL (e.g. "https://gitlab.example.com").
	endpoint string
	// token is the GitLab API access token.
	token string
	// projectID is the GitLab project ID.
	projectID int
	// ref is the Git ref (branch, tag, or commit SHA) to read policies from.
	// Defaults to "main".
	ref string `optgen:"default=\"main\""`
	// dir is the directory path within the repository containing policy files.
	dir string
	// includeData enables loading .json files as OPA data.
	includeData bool
	// logger sets the logger for the GitLab source.
	logger *slog.Logger
	// httpClientOptions are forwarded to the resilient HTTP client built
	// by the source. Set via the generated WithHTTPClientOptions (see
	// options_gen.go) — this is the single channel for configuring the
	// outbound transport (proxy, retry, breaker, custom transport).
	// Default behavior matches httpclient.New() defaults; pass
	// httpclient.WithRetryMax(0) etc. to opt out.
	httpClientOptions []httpclient.Option `opt:"HTTPClientOptions" optgen:"append"`
}
