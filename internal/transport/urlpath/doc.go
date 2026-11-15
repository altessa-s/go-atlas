// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package urlpath holds the one definition of a canonical HTTP request path
// shared by every HTTP middleware that selects a policy by path: ignore
// lists, per-path access rules.
//
// # Canonical form
//
// A path is canonical when a router dispatches on it as is: it starts with a
// slash and has no empty, "." or ".." segment. This is the form
// [net/http.ServeMux] and gorilla/mux clean a path to; both answer any other
// path with a redirect instead of a handler. A trailing slash is significant
// and stays: "/admin" and "/admin/" are two canonical paths.
//
// A router may match the escaped path while a policy reads the decoded one
// ([net/http.ServeMux] does, gorilla/mux with UseEncodedPath does). The two
// agree on the segments unless the escaped path holds an encoded slash, so a
// request path with "%2F" in it is not canonical either: "/v1%2Fhealthz" is
// "/v1/healthz" decoded and the single segment "v1/healthz" to the router.
//
// # Rule for policy lookups
//
// A policy is looked up for a canonical request path only. A path that is not
// canonical is never on an ignore list and is refused by the access-control
// middlewares, so neither "//admin" can miss the rule of "/admin" nor
// "/healthz/../admin" pass as the ignored "/healthz" prefix, nor
// "/v1%2Fhealthz" borrow the entry of "/v1/healthz".
//
//	if !urlpath.IsCanonicalURL(r.URL) {
//		http.Error(w, "Forbidden", http.StatusForbidden)
//		return
//	}
package urlpath
