// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"log/slog"
	"time"

	msdk "github.com/meilisearch/meilisearch-go"
)

// fakeSDK is a hand-rolled msdk.ServiceManager test double. The embedded
// interface field is nil — any method we don't override will panic on
// call, which surfaces an unmocked code path loudly instead of silently
// passing. The handful of methods we do override delegate to function
// fields the test sets up per-case; nil handlers fall back to a benign
// default (e.g. successful health probe with an empty body).
//
// Hand-rolled rather than imported from meilisearch-go/mocks because
// the mock package depends on stretchr/testify/mock which is not yet
// in our go.mod and pulls in a noticeably larger surface. The narrow
// fake here covers every method the Client uses today, and matches
// the project's existing pattern (see capturingRegistrar in
// data/outbox/scheduler_taskid_test.go).
type fakeSDK struct {
	msdk.ServiceManager

	healthFn      func(ctx context.Context) (*msdk.Health, error)
	indexFn       func(uid string) msdk.IndexManager
	getIndexFn    func(ctx context.Context, uid string) (*msdk.IndexResult, error)
	createIndexFn func(ctx context.Context, cfg *msdk.IndexConfig) (*msdk.TaskInfo, error)
	swapIndexesFn func(ctx context.Context, params []*msdk.SwapIndexesParams) (*msdk.TaskInfo, error)
	waitForTaskFn func(ctx context.Context, taskUID int64, interval time.Duration) (*msdk.Task, error)
}

func (f *fakeSDK) HealthWithContext(ctx context.Context) (*msdk.Health, error) {
	if f.healthFn != nil {
		return f.healthFn(ctx)
	}
	return &msdk.Health{Status: "available"}, nil
}

func (f *fakeSDK) Index(uid string) msdk.IndexManager {
	if f.indexFn != nil {
		return f.indexFn(uid)
	}
	return &fakeIndex{name: uid}
}

func (f *fakeSDK) GetIndexWithContext(ctx context.Context, uid string) (*msdk.IndexResult, error) {
	if f.getIndexFn != nil {
		return f.getIndexFn(ctx, uid)
	}
	return &msdk.IndexResult{UID: uid}, nil
}

func (f *fakeSDK) CreateIndexWithContext(ctx context.Context, cfg *msdk.IndexConfig) (*msdk.TaskInfo, error) {
	if f.createIndexFn != nil {
		return f.createIndexFn(ctx, cfg)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeSDK) SwapIndexesWithContext(ctx context.Context, params []*msdk.SwapIndexesParams) (*msdk.TaskInfo, error) {
	if f.swapIndexesFn != nil {
		return f.swapIndexesFn(ctx, params)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeSDK) WaitForTaskWithContext(ctx context.Context, taskUID int64, interval time.Duration) (*msdk.Task, error) {
	if f.waitForTaskFn != nil {
		return f.waitForTaskFn(ctx, taskUID, interval)
	}
	return &msdk.Task{Status: msdk.TaskStatusSucceeded, TaskUID: taskUID}, nil
}

// fakeIndex satisfies msdk.IndexManager the same way fakeSDK satisfies
// ServiceManager. Used for fluent calls like sdk.Index(name).AddDocuments(...).
type fakeIndex struct {
	msdk.IndexManager

	name string

	addDocumentsFn            func(ctx context.Context, docs any, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error)
	deleteDocumentFn          func(ctx context.Context, id string, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error)
	deleteDocumentsFn         func(ctx context.Context, ids []string, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error)
	deleteDocumentsByFilterFn func(ctx context.Context, filter any, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error)
	getDocumentsFn            func(ctx context.Context, q *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error
	searchFn                  func(ctx context.Context, query string, req *msdk.SearchRequest) (*msdk.SearchResponse, error)
	updateSettingsFn          func(ctx context.Context, settings *msdk.Settings) (*msdk.TaskInfo, error)
}

func (f *fakeIndex) AddDocumentsWithContext(ctx context.Context, docs any, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error) {
	if f.addDocumentsFn != nil {
		return f.addDocumentsFn(ctx, docs, opts)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndex) DeleteDocumentWithContext(ctx context.Context, id string, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error) {
	if f.deleteDocumentFn != nil {
		return f.deleteDocumentFn(ctx, id, opts)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndex) DeleteDocumentsWithContext(ctx context.Context, ids []string, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error) {
	if f.deleteDocumentsFn != nil {
		return f.deleteDocumentsFn(ctx, ids, opts)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndex) DeleteDocumentsByFilterWithContext(ctx context.Context, filter any, opts *msdk.DocumentOptions) (*msdk.TaskInfo, error) {
	if f.deleteDocumentsByFilterFn != nil {
		return f.deleteDocumentsByFilterFn(ctx, filter, opts)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

func (f *fakeIndex) GetDocumentsWithContext(ctx context.Context, q *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error {
	if f.getDocumentsFn != nil {
		return f.getDocumentsFn(ctx, q, resp)
	}
	resp.Total = 0
	return nil
}

func (f *fakeIndex) SearchWithContext(ctx context.Context, query string, req *msdk.SearchRequest) (*msdk.SearchResponse, error) {
	if f.searchFn != nil {
		return f.searchFn(ctx, query, req)
	}
	return &msdk.SearchResponse{}, nil
}

func (f *fakeIndex) UpdateSettingsWithContext(ctx context.Context, settings *msdk.Settings) (*msdk.TaskInfo, error) {
	if f.updateSettingsFn != nil {
		return f.updateSettingsFn(ctx, settings)
	}
	return &msdk.TaskInfo{TaskUID: 1}, nil
}

// newTestClient produces a *Client whose sdk field is the given fake.
// Skips the real msdk.New + initial health probe path of [New] so unit
// tests don't need a live Meilisearch instance. The httpClient is a
// no-op stub — Close() can be called without panic.
func newTestClient(sdk msdk.ServiceManager) *Client {
	return &Client{
		sdk:    sdk,
		logger: slog.New(slog.DiscardHandler),
	}
}
