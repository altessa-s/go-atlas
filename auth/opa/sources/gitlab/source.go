// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/auth/opa"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
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
)

// Source implements [opa.PolicySource] for GitLab-hosted policies.
// It fetches .rego files from a GitLab repository using the API.
// Change detection is handled by the Manager.
type Source struct {
	opts   *options
	logger *slog.Logger
	client *gitlabapi.Client

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

	logger := cmp.Or(o.logger, slog.New(slog.DiscardHandler))

	gitlabOpts := []gitlabapi.ClientOptionFunc{
		gitlabapi.WithBaseURL(strings.TrimRight(o.endpoint, "/") + "/api/v4"),
	}

	if o.retryMax > 0 {
		gitlabOpts = append(gitlabOpts,
			gitlabapi.WithHTTPClient(newResilientHTTPClient(o, logger)),
			gitlabapi.WithoutRetries(),
		)
	}

	client, err := gitlabapi.NewClient(o.token, gitlabOpts...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create gitlab client")
	}

	return &Source{
		opts:   o,
		logger: logger,
		client: client,
		shas:   make(map[string]string),
	}, nil
}

// newResilientHTTPClient builds a go-atlas HTTP client with retry support.
func newResilientHTTPClient(o *options, logger *slog.Logger) *http.Client {
	clientOpts := []httpclient.Option{
		httpclient.WithLogger(logger),
		httpclient.WithRetryMax(o.retryMax),
	}

	if o.retryWaitMin > 0 {
		clientOpts = append(clientOpts, httpclient.WithRetryWaitMin(o.retryWaitMin))
	}

	if o.retryWaitMax > 0 {
		clientOpts = append(clientOpts, httpclient.WithRetryWaitMax(o.retryWaitMax))
	}

	return httpclient.New(clientOpts...)
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

	entries, err := s.listTree(ctx)
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
			content, dlErr := s.downloadRawFile(ctx, name)
			if dlErr != nil {
				return nil, coreerrs.Wrapf(dlErr, "download policy %s", name)
			}

			modules[name] = content
			newSHAs[name] = entry.ID
		case s.opts.includeData && ext == ".json":
			content, dlErr := s.downloadRawFile(ctx, name)
			if dlErr != nil {
				return nil, coreerrs.Wrapf(dlErr, "download data %s", name)
			}

			var jsonData any
			if unmarshalErr := json.Unmarshal(content, &jsonData); unmarshalErr != nil {
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

	if len(modules) == 0 {
		return nil, coreerrs.Wrapf(opa.ErrNoPolicyFiles, "project %d path %s ref %s", s.opts.projectID, s.opts.dir, s.opts.ref)
	}

	s.shas = newSHAs

	var bundle *opa.PolicyBundle
	if data != nil {
		bundle = opa.NewPolicyBundleWithData(modules, data)
	} else {
		bundle = opa.NewPolicyBundle(modules)
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

// listTree retrieves the repository tree entries for the configured path with pagination.
func (s *Source) listTree(ctx context.Context) ([]*gitlabapi.TreeNode, error) {
	var all []*gitlabapi.TreeNode

	opts := &gitlabapi.ListTreeOptions{
		ListOptions: gitlabapi.ListOptions{Page: 1, PerPage: treePageSize},
		Ref:         gitlabapi.Ptr(s.opts.ref),
		Path:        gitlabapi.Ptr(s.opts.dir),
	}

	for {
		nodes, resp, err := s.client.Repositories.ListTree(s.opts.projectID, opts, gitlabapi.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list tree: %w", err)
		}

		all = append(all, nodes...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return all, nil
}

// downloadRawFile downloads a raw file from the GitLab repository.
func (s *Source) downloadRawFile(ctx context.Context, filename string) ([]byte, error) {
	filePath := path.Join(s.opts.dir, filename)

	data, _, err := s.client.RepositoryFiles.GetRawFile(
		s.opts.projectID,
		filePath,
		&gitlabapi.GetRawFileOptions{Ref: gitlabapi.Ptr(s.opts.ref)},
		gitlabapi.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", filename, err)
	}

	return data, nil
}

// Compile-time interface check.
var _ opa.PolicySource = (*Source)(nil)
