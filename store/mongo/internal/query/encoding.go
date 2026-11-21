// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"cmp"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type encoding struct {
	registry *bson.Registry
	opts     *options.BSONOptions
}

// The driver exposes the collection's client but not its BSON settings.
var clientEncodings sync.Map // *mongo.Client -> encoding

// RegisterClientEncoding records the merged options of a client created by Atlas.
func RegisterClientEncoding(client *mongo.Client, opts *options.ClientOptions) {
	clientEncodings.Store(client, encoding{registry: opts.Registry, opts: opts.BSONOptions})
}

// ForgetClientEncoding releases the settings when Atlas closes the client.
func ForgetClientEncoding(client *mongo.Client) {
	clientEncodings.Delete(client)
}

// FilterEncoding resolves explicit database or collection overrides, then
// client settings. External clients default to the Atlas registry.
func FilterEncoding(coll *mongo.Collection, registry *bson.Registry, opts *options.BSONOptions) (*bson.Registry, *options.BSONOptions) {
	if coll != nil && coll.Database() != nil {
		if value, ok := clientEncodings.Load(coll.Database().Client()); ok {
			client := value.(encoding) //nolint:errcheck // RegisterClientEncoding stores only encoding.
			registry = cmp.Or(registry, client.registry)
			opts = cmp.Or(opts, client.opts)
		}
	}
	if registry == nil {
		registry = Registry()
	}
	return registry, opts
}
