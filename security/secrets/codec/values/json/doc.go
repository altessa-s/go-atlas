// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package json provides a generic ValueDecoder implementation using JSON encoding.
//
// # Usage
//
//	decoder := json.NewValueDecoder[Config]()
//	config, err := decoder.Decode(data)
//	data, err := decoder.Encode(config)
package json
