// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package claimnum normalizes the numbers of a decoded JWT claim set.
//
// A number decoded with [json.Decoder.UseNumber] becomes a float64 when that
// value represents it exactly, so everything encoding/json would have
// returned as float64 keeps its type, and stays a [json.Number] otherwise, so
// two distinct integers above 2^53 never collapse into one value.
package claimnum
