// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the leader elector configuration to a NATS JetStream
// leader election provider for factory/coord/leadelect.
//
// The leadelect factory builds no provider itself; register this one for the
// nats type:
//
//	leader, err := leadelectfactory.New(cfg.LeaderElector).
//		UseProvider(lockconfig.LeaderElectorProviderNATS, natsfactory.Builder(natsConn)).
//		WithNodeId(nodeID).
//		Build(ctx)
package factory
