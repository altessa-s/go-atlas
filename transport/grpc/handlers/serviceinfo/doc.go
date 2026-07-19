// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package serviceinfo implements the gRPC ServiceInfoService
// (github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1).
//
// [Handler] returns runtime metadata for a service instance: name,
// description, identifiers, semantic version, build details, leader
// status, uptime, and arbitrary metadata. The static portion is built
// once in [New] from [appinfo] and protobuf-cloned on every request to
// keep callers isolated from each other.
//
// # Usage
//
//	h := serviceinfo.New(
//	    serviceinfo.WithServiceID(node.ID()),
//	    serviceinfo.WithServiceDescription("Customer billing API"),
//	    serviceinfo.WithLeaderProvider(electedLeader),
//	)
//	h.Register(grpcServer, stopCh)
//
// # Leader state
//
// Pass either a [LeaderProvider] (the [data/leadelect.Leader] type
// satisfies it directly) via [WithLeaderProvider], or a function via
// [WithLeader] when integrating a custom coordinator. The two options
// are mutually exclusive — the last call wins.
//
// # Security
//
// The handler performs NO authentication or authorization. It MUST be
// registered behind an auth interceptor or on a server bound to a
// non-public listener. Exposed publicly, it discloses service name,
// version, build details, instance identifiers, leader identity, and
// any metadata passed via [WithExtraMetadata] — reconnaissance data
// that lets an attacker map service topology and target known-version
// vulnerabilities.
package serviceinfo
