// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestNew_WithClient_And_ClientOptions_MutuallyExclusive(t *testing.T) {
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}

	_, err = New("testdb",
		WithClient(client),
		WithClientOptions(mongoOptions.Client().ApplyURI("mongodb://localhost:27017")),
	)
	if err == nil {
		t.Fatal("expected error when both WithClient and WithClientOptions are set")
	}

	const want = "WithClient and WithClientOptions are mutually exclusive"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestNew_WithClient_SetsClient(t *testing.T) {
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}

	m, err := New("testdb", WithClient(client))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if m.config.Client != client {
		t.Error("expected config.Client to be the provided client")
	}
}

func TestWithClient_Nil_Ignored(t *testing.T) {
	m, err := New("testdb", WithClient(nil))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if m.config.Client != nil {
		t.Error("expected config.Client to be nil when WithClient(nil) is used")
	}
}

func TestNew_WithoutClient_BackwardCompatible(t *testing.T) {
	m, err := New("testdb")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if m.config.Client != nil {
		t.Error("expected config.Client to be nil by default")
	}
}
