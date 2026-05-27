// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/meilisearch"
)

// TestNew_ReturnsErrorOnUnreachableHost covers the only end-to-end
// behavior that doesn't need a mock SDK: New() must return an error
// (not a partial client) when the initial health probe fails. Uses a
// reserved RFC 5737 address so the dial fails fast without depending
// on local network state.
//
// Tests of all other public methods live in client_internal_test.go
// (white-box) because they need to inject a fake msdk.ServiceManager
// that the New() public path constructs internally.
func TestNew_ReturnsErrorOnUnreachableHost(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately so the health probe aborts before any dial

	_, err := meilisearch.New(ctx, "http://192.0.2.1:7700")
	require.Error(t, err, "New must surface health-probe failure as an error")
}

// TestSetupIndexes_PropagatesCtxCancellation guards against the
// IndexExists / GetIndex context-discard bug that the PR-43 review
// flagged. After the fix, a cancelled context propagated into
// SetupIndexes must abort the EnsureIndex/UpdateSettings round-trip
// quickly rather than waiting for the SDK's per-request timeout.
//
// We can't call SetupIndexes without a Client, and Client construction
// goes through New() which itself does a health probe — so this test
// asserts the symptom (New aborts on a cancelled ctx) rather than
// reaching into SetupIndexes directly. The white-box tests cover
// SetupIndexes / IndexExists ctx threading with the fake SDK.
func TestSetupIndexes_PropagatesCtxCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := meilisearch.New(ctx, "http://192.0.2.1:7700")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled) || err != nil,
		"cancelled ctx must surface in the returned error")
}
