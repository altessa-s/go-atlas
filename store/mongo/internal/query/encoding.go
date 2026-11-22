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

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
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
	var client *mongo.Client
	if coll != nil && coll.Database() != nil {
		client = coll.Database().Client()
	}
	settings := clientEncoding(client)
	if registry = cmp.Or(registry, settings.registry); registry == nil {
		registry = Registry()
	}
	return registry, cmp.Or(opts, settings.opts)
}

// ClientRegistry reuses the registry of an Atlas-created driver client.
// A new client or an unregistered external client receives a private Atlas registry.
func ClientRegistry(client *mongo.Client) *bson.Registry {
	if reg := clientEncoding(client).registry; reg != nil {
		return reg
	}
	return bsoncodec.NewRegistry()
}

func clientEncoding(client *mongo.Client) encoding {
	value, _ := clientEncodings.Load(client)
	settings, _ := value.(encoding)
	return settings
}
