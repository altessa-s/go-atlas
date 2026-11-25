// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsonfield

import "errors"

// ErrFieldTransform identifies a field that the builder could not transform.
var ErrFieldTransform = errors.New("cannot transform BSON field")
