// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import "slices"

// WithAllowedAlgorithms sets the signature algorithms the verifier accepts,
// replacing the default asymmetric allow-list ([DefaultAllowedAlgorithms]).
// Restricting or extending the set is the supported way to add support for a
// new algorithm: register a signing method with golang-jwt, then list it here.
// A call with no algorithms is ignored so the safe default is never cleared.
func WithAllowedAlgorithms(algs ...Algorithm) Option {
	return func(o *options) {
		if len(algs) == 0 {
			return
		}
		o.allowedAlgorithms = slices.Clone(algs)
	}
}
