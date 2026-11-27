// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package s3probe checks that an S3-compatible bucket is reachable with a
// given pair of access keys.
//
// The single operation is a signed HeadBucket request with path-style
// addressing, sent through the guarded HTTP client of go-atlas-net. The result
// is the HTTP status the service answered with, or a [TransportError] when no
// response was received. Mapping a status to a reason is the caller's job: the
// package never assigns two meanings to one status.
//
// # Egress
//
// A trusted target (a stored configuration) is checked under the egress policy
// the [Prober] was built with: a private, loopback, link-local or reserved
// address is reachable only through an allow rule, and a deny rule wins. An
// untrusted target (a draft) is checked under the default protection alone, so
// only public addresses are reachable whatever the policy allows. A refusal by
// either policy happens before connecting and is reported as [KindPolicy],
// which is distinct from an unreachable endpoint.
//
// # Usage
//
//	p := s3probe.New(policy)
//
//	res, err := p.HeadBucket(ctx, s3probe.Request{
//	    Endpoint:  "https://s3.example.com",
//	    Bucket:    "reports",
//	    AccessKey: "AKIA...",
//	    SecretKey: secret,
//	    Region:    "eu-west-1",
//	    Timeout:   5 * time.Second,
//	    Trusted:   true,
//	})
//	switch {
//	case errors.Is(err, s3probe.ErrPolicyRefused):
//	    // refused before connecting
//	case err != nil:
//	    // unreachable, TLS failure or timeout
//	default:
//	    _ = res.StatusCode
//	}
package s3probe
