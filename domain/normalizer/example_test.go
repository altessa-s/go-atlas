// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"fmt"

	"github.com/altessa-s/go-atlas/domain/normalizer"
)

// ExampleNormalize demonstrates the tag-based normalization system: only
// fields carrying a `normalize` tag are processed, everything else is left
// untouched.
func ExampleNormalize() {
	type User struct {
		Name        string   `normalize:"trim,lowercase"`          // Trimmed and lowercased.
		Email       *string  `normalize:"trim,nil_on_empty"`       // Trimmed, nil if empty.
		Phone       string   `normalize:"phone(region=US)"`        // Normalized to E.164 format.
		Description string   `normalize:"trim,remove_bad_symbols"` // Trimmed and cleaned.
		Tags        []string `normalize:"remove_empty_elements"`   // Empty strings removed.
		Age         int      // No tag — ignored.
		IgnoreThis  string   `normalize:"-"` // Explicitly ignored.
	}

	email := "  user@example.com  "
	user := &User{
		Name:        "  John Doe  ",
		Email:       &email,
		Phone:       "(212) 555-1234",
		Description: "  Some text with\u0000control chars  ",
		Tags:        []string{"tag1", "", "tag2", ""},
		Age:         30,
		IgnoreThis:  "  KEEP AS IS  ",
	}

	if err := normalizer.Normalize(user); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Printf("%q\n", user.Name)
	fmt.Printf("%q\n", *user.Email)
	fmt.Printf("%q\n", user.Phone)
	fmt.Printf("%q\n", user.Description)
	fmt.Printf("%q\n", user.Tags)
	fmt.Println(user.Age)
	fmt.Printf("%q\n", user.IgnoreThis)

	// Output:
	// "john doe"
	// "user@example.com"
	// "+12125551234"
	// "Some text withcontrol chars"
	// ["tag1" "tag2"]
	// 30
	// "  KEEP AS IS  "
}

// Person demonstrates combining tag-based normalization with a custom
// Normalize method: the method runs after the tag-based pass and can compute
// derived fields.
type Person struct {
	FirstName string `normalize:"trim"`
	LastName  string `normalize:"trim"`
	FullName  string // Computed by Normalize.
}

// Normalize implements the CustomNormalizer interface.
func (p *Person) Normalize() error {
	p.FullName = p.FirstName + " " + p.LastName
	return nil
}

func ExampleNormalize_customNormalizer() {
	person := &Person{
		FirstName: "  John  ",
		LastName:  "  Doe  ",
	}

	if err := normalizer.Normalize(person); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Printf("%q %q %q\n", person.FirstName, person.LastName, person.FullName)

	// Output:
	// "John" "Doe" "John Doe"
}

// ExampleNormalize_parameterizedModifiers demonstrates modifiers that accept
// parameters, such as the parsing region of the phone modifier.
func ExampleNormalize_parameterizedModifiers() {
	type ContactInfo struct {
		PhoneUS string `normalize:"phone(region=US)"`      // US region for parsing.
		PhoneDE string `normalize:"phone(region=DE)"`      // German region for parsing.
		Mixed   string `normalize:"trim,phone(region=GB)"` // Regular + parameterized modifiers.
	}

	contact := &ContactInfo{
		PhoneUS: "(212) 555-1234",
		PhoneDE: "030 12345678",
		Mixed:   "  020 7946 0958  ",
	}

	if err := normalizer.Normalize(contact); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(contact.PhoneUS)
	fmt.Println(contact.PhoneDE)
	fmt.Println(contact.Mixed)

	// Output:
	// +12125551234
	// +493012345678
	// +442079460958
}
