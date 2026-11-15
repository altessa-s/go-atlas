// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

//go:generate go tool optgen generate

// options configures an [InProcessBus] at construction.
type options struct {
	// inTx ([WithTxProbe]) reports an active transaction in ctx. It is the
	// only place a concrete backend is wired in; the bus core never imports a
	// database driver. Immutable after [New].
	inTx TxProbe `opt:"TxProbe"`
}
