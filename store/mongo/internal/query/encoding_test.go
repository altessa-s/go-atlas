// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/store/mongo/internal/query"
)

func TestFilterEncoding(t *testing.T) {
	t.Parallel()
	registry := bson.NewRegistry()
	opts := options.Client().SetRegistry(registry).SetBSONOptions(&options.BSONOptions{IntMinSize: true})
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Disconnect(t.Context())) }()
	coll := client.Database("db").Collection("items")
	query.RegisterClientEncoding(client, opts)
	t.Cleanup(func() { query.ForgetClientEncoding(client) })
	reg, enc := query.FilterEncoding(coll, nil, nil)
	require.Same(t, registry, reg)
	require.Same(t, opts.BSONOptions, enc)

	override := bson.NewRegistry()
	reg, enc = query.FilterEncoding(coll, override, nil)
	require.Same(t, override, reg)
	require.Same(t, opts.BSONOptions, enc)
	query.ForgetClientEncoding(client)
	reg, enc = query.FilterEncoding(coll, nil, nil)
	require.Same(t, query.Registry(), reg)
	require.Nil(t, enc)
}
