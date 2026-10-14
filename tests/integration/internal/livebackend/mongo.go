// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend

import "os"

// DefaultMongoPort is the host port of the MongoDB service in
// tests/integration/docker-compose.yml when MONGO_PORT is not set.
const DefaultMongoPort = "27019"

// MongoURI returns MONGO_URI when set, otherwise the compose replica set on
// 127.0.0.1 at MONGO_PORT (default [DefaultMongoPort]) — the same variable the
// compose file publishes the port under.
//
// directConnection is required: the compose service is a single-node replica
// set that advertises the address it sees inside its own container, so a
// driver doing topology discovery from the host would follow that
// advertisement to a port nothing listens on.
func MongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	port := os.Getenv("MONGO_PORT")
	if port == "" {
		port = DefaultMongoPort
	}
	return "mongodb://127.0.0.1:" + port + "/?directConnection=true"
}
