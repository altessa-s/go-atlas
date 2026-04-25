// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

// Person is a common test struct with Name and Age.
type Person struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// User is a common test struct with ID and Name.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
