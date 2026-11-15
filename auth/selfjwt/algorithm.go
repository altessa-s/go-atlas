// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import "github.com/altessa-s/go-atlas/auth/jwt"

// defaultAllowedAlgorithms is the verifier's fail-closed default allow-list:
// asymmetric algorithms only. Referenced by the generated options defaults.
var defaultAllowedAlgorithms = []jwt.Algorithm{jwt.AlgEdDSA, jwt.AlgES256, jwt.AlgRS256}
