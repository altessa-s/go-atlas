// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/internal/checksum"
	"github.com/altessa-s/go-atlas/auth/opa/internal/jsondoc"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	gitlabapi "gitlab.com/gitlab-org/api/client-go"
)

// treePageSize is the number of items per page when listing the repository tree.
const treePageSize = 100

// Sentinel errors.
var (
	// ErrEndpointRequired is returned when no GitLab endpoint is configured.
	ErrEndpointRequired = errors.New("gitlab endpoint is required")
	// ErrTokenRequired is returned when no GitLab access token is configured.
	ErrTokenRequired = errors.New("gitlab token is required")
	// ErrProjectIDRequired is returned when no GitLab project ID is configured.
	ErrProjectIDRequired = errors.New("gitlab project ID is required")
	// ErrHTTPClientRequired is returned when no client is passed with
	// [WithHTTPClient]. Pass the resilient, SSRF-protected client from
	// net/http/client (httpclient.New()), as the factory does.
	ErrHTTPClientRequired = errors.New("gitlab HTTP client is required")
	// ErrInsecureEndpoint is returned when the endpoint is not an https URL
	// and [WithAllowInsecureHTTP] is not set.
	ErrInsecureEndpoint = errors.New("gitlab endpoint must use https")
	// ErrChecksumMismatch is returned when a fetched file's SHA-256 digest
	// does not match its [WithChecksums] entry.
	ErrChecksumMismatch = errors.New("gitlab policy file checksum mismatch")
	// ErrUnexpectedPolicyFile is returned when a fetched file has no
	// [WithChecksums] entry.
	ErrUnexpectedPolicyFile = errors.New("gitlab policy file not in checksums manifest")
	// ErrMissingPolicyFile is returned when a [WithChecksums] entry has no
	// fetched file.
	ErrMissingPolicyFile = errors.New("gitlab policy file in checksums manifest not found")
	// ErrFileTooLarge is returned when a file exceeds [WithMaxFileBytes].
	ErrFileTooLarge = errors.New("gitlab policy file too large")
)

// Source implements [opa.PolicySource] for GitLab-hosted policies.
// It fetches .rego files from a GitLab repository using the API.
// Change detection is handled by the Manager.
type Source struct {
	opts      *options
	logger    *slog.Logger
	client    *gitlabapi.Client
	checksums *checksum.Manifest

	mu     sync.Mutex
	shas   map[string]string // filename -> blob SHA
	closed bool
}

// New creates a new GitLab policy source.
func New(opts ...Option) (*Source, error) {
	o := newOptions(opts...)

	if o.endpoint == "" {
		return nil, ErrEndpointRequired
	}

	if o.token == "" {
		return nil, ErrTokenRequired
	}

	if o.projectID == 0 {
		return nil, ErrProjectIDRequired
	}

	if o.httpClient == nil {
		return nil, ErrHTTPClientRequired
	}

	if err := checkEndpointScheme(o.endpoint, o.allowInsecureHTTP); err != nil {
		return nil, err
	}

	manifest := checksum.New(o.checksums)
	o.checksums = nil

	logger := cmp.Or(o.logger, slog.New(slog.DiscardHandler))
	if manifest == nil {
		logger.Warn("gitlab policy source has no checksums manifest: fetched policies are not integrity-verified",
			slog.Int("project", o.projectID),
			slog.String("ref", o.ref))
	}

	// gitlab's library-level retry is disabled so the injected client is
	// the only retry layer.
	gitlabOpts := []gitlabapi.ClientOptionFunc{
		gitlabapi.WithBaseURL(strings.TrimRight(o.endpoint, "/") + "/api/v4"),
		gitlabapi.WithHTTPClient(o.httpClient),
		gitlabapi.WithoutRetries(),
	}

	client, err := gitlabapi.NewClient(o.token, gitlabOpts...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create gitlab client")
	}

	return &Source{
		opts:      o,
		logger:    logger,
		client:    client,
		checksums: manifest,
		shas:      make(map[string]string),
	}, nil
}

// checkEndpointScheme requires an https endpoint, or http with allowHTTP.
func checkEndpointScheme(endpoint string, allowHTTP bool) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return coreerrs.Wrapf(ErrInsecureEndpoint, "parse %q: %v", endpoint, err)
	}
	switch {
	case u.Scheme == "https" && u.Host != "":
		return nil
	case u.Scheme == "http" && u.Host != "" && allowHTTP:
		return nil
	default:
		return coreerrs.Wrapf(ErrInsecureEndpoint, "%q", endpoint)
	}
}

// Name returns the source identifier.
func (s *Source) Name() string {
	return fmt.Sprintf("gitlab:%d/%s@%s", s.opts.projectID, s.opts.dir, s.opts.ref)
}

// Fetch retrieves the current policy bundle from the GitLab repository.
func (s *Source) Fetch(ctx context.Context) (*opa.PolicyBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, opa.ErrSourceClosed
	}

	ref, err := s.resolveRef(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.listTree(ctx, ref)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "list repository tree")
	}

	modules := make(map[string][]byte)
	var data map[string]any
	newSHAs := make(map[string]string)

	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}

		name := entry.Name
		ext := path.Ext(name)

		switch {
		case ext == ".rego":
			content, dlErr := s.downloadVerified(ctx, ref, name)
			if dlErr != nil {
				return nil, coreerrs.Wrapf(dlErr, "download policy %s", name)
			}

			modules[name] = content
			newSHAs[name] = entry.ID
		case s.opts.includeData && ext == ".json":
			content, dlErr := s.downloadVerified(ctx, ref, name)
			if dlErr != nil {
				return nil, coreerrs.Wrapf(dlErr, "download data %s", name)
			}

			jsonData, unmarshalErr := jsondoc.Decode(content)
			if unmarshalErr != nil {
				return nil, coreerrs.Wrapf(unmarshalErr, "parse JSON data %s", name)
			}

			if data == nil {
				data = make(map[string]any)
			}

			key := strings.TrimSuffix(name, ".json")
			data[key] = jsonData
			newSHAs[name] = entry.ID
		}
	}

	if err := s.checksums.Covered(newSHAs, ErrMissingPolicyFile); err != nil {
		return nil, err
	}

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "project %d path %s ref %s", s.opts.projectID, s.opts.dir, s.opts.ref)
	}

	s.shas = newSHAs

	bundle, revisionErr := opa.NewPolicyBundleWithData(modules, data)
	if revisionErr != nil {
		return nil, revisionErr
	}

	s.logger.Debug("fetched policy bundle from gitlab",
		slog.Int("project", s.opts.projectID),
		slog.String("ref", s.opts.ref),
		slog.Int("modules", len(modules)),
		slog.String("revision", bundle.Revision))

	return bundle, nil
}

// Close releases resources held by the source.
func (s *Source) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	s.logger.Debug("gitlab source closed",
		slog.Int("project", s.opts.projectID))

	return nil
}

func (s *Source) resolveRef(ctx context.Context) (string, error) {
	commit, response, err := s.client.Commits.GetCommit(s.opts.projectID, s.opts.ref, nil, gitlabapi.WithContext(ctx))
	if err != nil {
		if response != nil && response.Response != nil {
			return "", fmt.Errorf("resolve policy ref (HTTP %d)", response.StatusCode)
		}
		return "", coreerrs.WrapOperation(err, "resolve policy ref")
	}
	if commit == nil {
		return "", errors.New("missing GitLab commit")
	}
	sha, err := hex.DecodeString(commit.ID)
	if err != nil || (len(sha) != 20 && len(sha) != 32) {
		return "", errors.New("invalid GitLab commit ID")
	}
	return commit.ID, nil
}

// listTree retrieves the repository tree entries for the configured path with pagination.
func (s *Source) listTree(ctx context.Context, ref string) ([]*gitlabapi.TreeNode, error) {
	var all []*gitlabapi.TreeNode

	opts := &gitlabapi.ListTreeOptions{
		Page: 1, PerPage: treePageSize,
		Ref:  &ref,
		Path: new(s.opts.dir),
	}

	for {
		nodes, resp, err := s.client.Repositories.ListTree(s.opts.projectID, opts, gitlabapi.WithContext(ctx))
		if err != nil {
			return nil, coreerrs.Wrap(err, "list tree")
		}

		all = append(all, nodes...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return all, nil
}

// downloadVerified downloads a file and checks it against the checksums
// manifest, when one is configured.
func (s *Source) downloadVerified(ctx context.Context, ref, filename string) ([]byte, error) {
	content, err := s.downloadRawFile(ctx, ref, filename)
	if err != nil {
		return nil, err
	}
	if err := s.checksums.Verify(filename, content, ErrChecksumMismatch, ErrUnexpectedPolicyFile); err != nil {
		return nil, err
	}
	return content, nil
}

// downloadRawFile downloads a raw file from the GitLab repository, reading at
// most maxFileBytes of it.
func (s *Source) downloadRawFile(ctx context.Context, ref, filename string) ([]byte, error) {
	filePath := path.Join(s.opts.dir, filename)

	req, err := s.client.NewRequest(http.MethodGet,
		fmt.Sprintf("projects/%d/repository/files/%s/raw", s.opts.projectID, gitlabapi.PathEscape(filePath)),
		&gitlabapi.GetRawFileOptions{Ref: &ref},
		[]gitlabapi.RequestOptionFunc{gitlabapi.WithContext(ctx)})
	if err != nil {
		return nil, coreerrs.Wrapf(err, "build request for %s", filename)
	}

	buf := &cappedBuffer{limit: s.opts.maxFileBytes}
	if _, err := s.client.Do(req, buf); err != nil {
		return nil, coreerrs.Wrapf(err, "download %s", filename)
	}

	return buf.data, nil
}

// cappedBuffer collects a response body and fails once it exceeds limit
// bytes, so an oversized file is never held in memory in full.
type cappedBuffer struct {
	data  []byte
	limit int64
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if int64(len(b.data))+int64(len(p)) > b.limit {
		return 0, coreerrs.Wrapf(ErrFileTooLarge, "exceeds %d bytes", b.limit)
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
