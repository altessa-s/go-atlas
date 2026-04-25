// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package natsbase provides a base type for NATS JetStream KeyValue providers.
//
// The Base type encapsulates the common pattern of storing and accessing
// a jetstream.KeyValue store. Providers embed Base to gain access to
// the underlying KeyValue store via the KV() method.
//
// Example usage:
//
//	type Provider struct {
//	    natsbase.Base
//	    opts *options
//	}
//
//	func New(js jetstream.JetStream, opts ...Option) (*Provider, error) {
//	    base, err := natsbase.NewBaseWithBucket(ctx, js, natskvlease.BucketConfig{
//	        Bucket: "my-bucket",
//	        TTL:    time.Hour,
//	    }, nil)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &Provider{Base: base, opts: newOptions(opts...)}, nil
//	}
//
//	func (p *Provider) Get(ctx context.Context, key string) ([]byte, error) {
//	    entry, err := p.KV().Get(ctx, key)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return entry.Value(), nil
//	}
package natsbase
