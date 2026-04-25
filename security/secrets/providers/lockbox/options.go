// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/codec"

	_ "github.com/altessa-s/go-atlas/security/secrets/codec/keys/base64"
	_ "github.com/altessa-s/go-atlas/security/secrets/codec/values/json"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// options contains configuration settings for the Lockbox storage.
// It is configured through the functional options passed to New.
type options[T any] struct {
	labels               map[string]string
	ignoreInvalidKeys    bool
	valueDecoder         codec.ValueDecoder[T]            `optgen:"default=json.NewValueDecoder[T](),notnil"`
	keyDecoder           codec.KeyDecoder                 `optgen:"default=base64.NewKeyDecoder(),notnil"`
	locker               secrets.Locker                   `optgen:"notnil"`
	concurrencyLimitFunc concurrency.ConcurrencyLimitFunc `optgen:"notnil"`
}
