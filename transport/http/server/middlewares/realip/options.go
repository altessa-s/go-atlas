// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

//go:generate go tool optgen generate --type=options

type options struct {
	// logClientDetails includes the remote address in parse failure logs.
	logClientDetails bool `optval:"param"`
	// logRawErrors includes the underlying parse error, which can contain the address.
	logRawErrors bool `optval:"param"`
}
