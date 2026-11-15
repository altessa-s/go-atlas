// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package principalowner adapts authenticated principals to data idempotency
// ownership checks.
//
// # Principal ownership
//
// [PrincipalOwner] returns an owner callback that binds a record to the
// verified principal's tenant and subject. Use it with the owner option when
// constructing a keeper directly; the idempotency factory uses it as its
// default.
//
// # Usage
//
//	import (
//	    "github.com/altessa-s/go-atlas/reliability/idempotency/principalowner"
//	    dataidempotency "github.com/altessa-s/go-atlas/reliability/idempotency"
//	)
//	keeper := dataidempotency.New(storage,
//	    dataidempotency.WithOwner(principalowner.PrincipalOwner),
//	)
package principalowner
