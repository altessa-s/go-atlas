// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package urlpath

import (
	"net/url"
	"strings"
)

// IsCanonicalURL reports whether the request path of u selects the same
// endpoint for a policy lookup on the decoded path and for a router matching
// the escaped one: the decoded path is canonical ([IsCanonical]) and the
// escaped path carries no encoded slash (%2F, either case).
//
// [net/http.ServeMux], and gorilla/mux with UseEncodedPath, match segments of
// the escaped path, so "/v1%2Fhealthz" is one segment for the router and two
// for the decoded path. Without an encoded slash both forms split into the
// same segments, and an encoded dot segment ("%2E%2E") shows up as a dot
// segment of the decoded path. Any other escape, an encoded backslash
// included, is an ordinary character to both. The check does not allocate;
// a path that needed no special encoding (empty RawPath) costs no second scan.
func IsCanonicalURL(u *url.URL) bool {
	return IsCanonical(u.Path) && (u.RawPath == "" || !hasEncodedSlash(u.RawPath))
}

func hasEncodedSlash(escaped string) bool {
	for i := 0; i+2 < len(escaped); i++ {
		if escaped[i] == '%' && escaped[i+1] == '2' && (escaped[i+2] == 'F' || escaped[i+2] == 'f') {
			return true
		}
	}
	return false
}

// IsCanonical reports whether p is the path a router dispatches on as is: it
// starts with a slash and has no empty, "." or ".." segment. A single
// trailing slash is allowed and significant. Pass the decoded path
// ([net/url.URL.Path]), so an escaped separator or dot segment is seen as
// what it decodes to. The check does not allocate. For a request use
// [IsCanonicalURL], which also sees how the path was escaped.
func IsCanonical(p string) bool {
	if p == "" || p[0] != '/' {
		return false
	}
	for rest := p[1:]; rest != ""; {
		segment, tail, more := strings.Cut(rest, "/")
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		if !more {
			return true
		}
		rest = tail
	}
	return true
}
