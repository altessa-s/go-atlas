// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package natskeytest holds the logical keys the NATS KeyValue store tests
// push through [natskey.Encode]: keys the server rejects verbatim, and a
// pair that would collide under a naive escaping.
//
//	for _, key := range natskeytest.Keys() {
//	    _, _, _, err := storage.AttemptLock(ctx, key, []byte(key))
//	    require.NoError(t, err, key)
//	}
package natskeytest
