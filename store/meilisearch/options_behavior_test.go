// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"net/http"
	"testing"
)

func TestWithHTTPClientBehavior(t *testing.T) {
	opts := defaultOptions()
	defaultTimeout := opts.timeout
	WithHTTPClient(nil)(opts)
	if opts.httpClient != nil {
		t.Fatal("nil client changed the default client")
	}

	client := &http.Client{}
	option := WithHTTPClient(client)
	client.Timeout = 3
	option(opts)
	if opts.httpClient != client {
		t.Fatal("client pointer was copied or not applied")
	}
	client.Timeout = 5
	if opts.httpClient.Timeout != 5 {
		t.Fatal("mutating client after applying did not affect stored client")
	}
	if opts.timeout != defaultTimeout || opts.apiKey != "" {
		t.Fatal("WithHTTPClient changed unrelated defaults")
	}
}
