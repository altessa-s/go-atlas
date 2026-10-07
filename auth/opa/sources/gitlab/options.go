// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"net/http"
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
	// httpClient serves every GitLab API call. Required: New fails with
	// ErrHTTPClientRequired without it. The factory injects the resilient,
	// SSRF-protected atlas client (retry, breaker, proxy) built from
	// configuration.
	httpClient *http.Client `opt:"HTTPClient"`
}
