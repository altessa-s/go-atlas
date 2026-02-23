// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a MongoDB client wrapper with CSFLE encryption,
// transaction handling, and structured logging. Use it for database operations
// requiring encryption or simplified transaction management.
//
// Example:
//
//	client, err := mongo.New("myapp",
//	    mongo.WithMongoClientOptions(options.Client().ApplyURI(uri)),
//	    mongo.WithLogger(logger),
//	)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	if err := client.Connect(ctx); err != nil {
//	    return err
//	}
package mongo
