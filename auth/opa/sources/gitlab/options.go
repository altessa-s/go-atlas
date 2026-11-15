// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"net/http"
)

// DefaultMaxFileBytes caps how much of one repository file is read into
// memory, matching the S3 source's per-object cap.
const DefaultMaxFileBytes int64 = 32 << 20 // 32 MiB

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
	// allowInsecureHTTP permits a plain http:// endpoint. Off by default: the
	// access token and the policies would cross the network unprotected.
	allowInsecureHTTP bool `opt:"AllowInsecureHTTP"`
	// checksums maps policy and data file names (relative to dir) to their
	// expected SHA-256 hex digests. When set, every fetched file must match
	// its entry and every entry must be fetched. Nil disables verification,
	// which New reports once with a warning.
	checksums map[string]string `optgen:"default=nil"`
	// maxFileBytes caps the size of one downloaded file.
	maxFileBytes int64 `optgen:"default=DefaultMaxFileBytes" optval:"positive"`
}
