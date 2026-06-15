// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package etag produces and compares entity-tags (ETags) as defined by
// RFC 7232, in both strong (`"value"`) and weak (`W/"value"`) forms. It is
// transport-neutral: the same tags drive HTTP conditional requests and the
// resource `etag` field used by gRPC APIs that follow Google AIP-154, which
// reuses the RFC 7232 syntax verbatim as the field value.
//
// A [Tag] stores the inner opaque-tag without its surrounding quotes plus a
// weak flag. [Tag.String] renders the wire form — equally valid as an HTTP
// ETag header or an AIP-154 `etag` string field — and [Parse]/[ParseList]
// recover a [Tag] (or an If-None-Match/If-Match list) from that form.
// Comparison follows RFC 7232 §2.3.2: [Tag.StrongMatch] requires both tags to
// be strong, while [Tag.WeakMatch] ignores weakness.
//
// Strong content tags are produced by a [Generator]. Its hash is pluggable
// (default SHA-256 via [DefaultNewHash]) so callers may trade collision
// resistance for speed when uniqueness guarantees allow:
//
//   - [Generator.Hash] / [Generator.HashString] hash an in-memory body.
//   - [Generator.HashReader] streams a reader without buffering it whole.
//
// [FromModTime] builds a weak validator from object metadata in the
// nginx/Apache `size-mtime` style, without hashing at all.
//
// # Transports
//
// This package is a generation primitive only. HTTP carries the tag in the
// ETag / If-Match / If-None-Match headers; AIP-154 carries it in a resource
// `etag` string field and validates a mutation by strong-comparing the
// request `etag` against the current one, returning ABORTED on mismatch.
// Either way the value comes from [Tag.String] and is read back by [Parse];
// the header-, 304-, and status-code wiring lives in the transport adapters.
//
// # Usage
//
//	g := etag.NewGenerator()
//	tag := g.HashString(body)         // `"<sha256-hex>"`
//	w.Header().Set("ETag", tag.String())
//
//	if inm, star, err := etag.ParseList(r.Header.Get("If-None-Match")); err == nil {
//	    for _, c := range inm {
//	        if star || c.WeakMatch(tag) {
//	            w.WriteHeader(http.StatusNotModified)
//	            return
//	        }
//	    }
//	}
//
// gRPC / AIP-154 — validate a mutation, then stamp the response:
//
//	cur := g.HashString(resource)
//	if req.GetEtag() != "" {
//	    want, err := etag.Parse(req.GetEtag())
//	    if err != nil || !want.StrongMatch(cur) {
//	        return nil, status.Error(codes.Aborted, "etag mismatch")
//	    }
//	}
//	resp.Etag = cur.String()
package etag
