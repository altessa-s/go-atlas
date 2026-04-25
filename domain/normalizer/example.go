// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

const (
	// DefaultAge represents a sample age for documentation examples
	DefaultAge = 30
)

// Example demonstrates the tag-based normalization system.
// This example shows how to use normalize tags in struct fields.
func Example() {
	// Example struct usage with normalize tags:
	type User struct {
		Name        string    `normalize:"trim,lowercase"`          // Will be trimmed and lowercased
		Email       *string   `normalize:"trim,nil_on_empty"`       // Will be trimmed and set to nil if empty
		Phone       string    `normalize:"phone"`                   // Will be normalized to E164 format
		Description string    `normalize:"trim,remove_bad_symbols"` // Will be trimmed and cleaned
		Tags        []string  `normalize:"remove_empty_elements"`   // Empty strings will be removed
		Notes       []*string `normalize:"remove_empty_elements"`   // nil and empty will be removed
		CustomField string    `normalize:"custom"`                  // Only custom Normalize() called, no modifiers
		Age         int       // No tag - will be ignored
		IgnoreThis  string    `normalize:"-"` // Explicitly ignored
	}

	name := "  John Doe  "
	email := "  JOHN@EXAMPLE.COM  "
	note1 := "Note 1"
	emptyNote := ""

	user := &User{
		Name:        name,
		Email:       &email,
		Phone:       "8 (999) 450-06-45",                     // Will be normalized to +79994500645
		Description: "  Some text with\u0000control chars  ", // Control chars will be removed
		Tags:        []string{"tag1", "", "tag2", ""},        // Empty strings will be removed
		Notes:       []*string{&note1, nil, &emptyNote, nil}, // nil and empty will be removed
		CustomField: "  CUSTOM FIELD  ",                      // Will be processed only by custom Normalize()
		Age:         DefaultAge,                              // Will be ignored (no tag)
		IgnoreThis:  "  KEEP AS IS  ",                        // Will be ignored (tag: "-")
	}

	// Normalize will process only fields with normalize tags
	if err := Normalize(user); err != nil {
		panic(err)
	}

	// Results after normalization:
	// - user.Name: "john doe" (trimmed and lowercased)
	// - user.Email: "john@example.com" or nil if was empty (trimmed and nil_on_empty)
	// - user.Phone: "+79994500645" (normalized to E164 format)
	// - user.Description: "Some text withcontrol chars" (trimmed and control chars removed)
	// - user.Tags: ["tag1", "tag2"] (empty strings removed)
	// - user.Notes: [&"Note 1"] (nil and empty removed)
	// - user.CustomField: "  CUSTOM FIELD  " (unchanged - processed only by custom Normalize())
	// - user.Age: DefaultAge (unchanged - no tag)
	// - user.IgnoreThis: "  KEEP AS IS  " (unchanged - tag: "-")
}

// Person demonstrates custom normalization with tags.
type Person struct {
	FirstName string `normalize:"trim"` // Tag-based normalization
	LastName  string `normalize:"trim"` // Tag-based normalization
	FullName  string // Will be computed in Normalize() method
}

// Normalize implements CustomNormalizer interface
func (p *Person) Normalize() error {
	// This will be called after tag-based normalization
	p.FullName = p.FirstName + " " + p.LastName
	return nil
}

// ExampleCustomNormalization demonstrates custom normalization with tags.
func ExampleCustomNormalization() {
	person := &Person{
		FirstName: "  John  ",
		LastName:  "  Doe  ",
	}

	if err := Normalize(person); err != nil {
		panic(err)
	}

	// Results:
	// - person.FirstName: "John" (trimmed by tag)
	// - person.LastName: "Doe" (trimmed by tag)
	// - person.FullName: "John Doe" (computed by custom Normalize())
}

// ContactInfo demonstrates parameterized modifiers.
type ContactInfo struct {
	PhoneRU    string `normalize:"phone"`                 // Uses default region (RU)
	PhoneUS    string `normalize:"phone(region=US)"`      // Uses US region for parsing
	PhoneDE    string `normalize:"phone(region=DE)"`      // Uses German region for parsing
	MixedField string `normalize:"trim,phone(region=GB)"` // Mixed: regular + parameterized modifiers
}

// ExampleParameterizedModifiers demonstrates the use of parameterized modifiers.
func ExampleParameterizedModifiers() {
	contact := &ContactInfo{
		PhoneRU:    "8 (999) 123-45-67", // Russian format
		PhoneUS:    "(212) 555-1234",    // US format (NYC area)
		PhoneDE:    "030 12345678",      // German format (Berlin)
		MixedField: "  020 7946 0958  ", // UK format with spaces
	}

	if err := Normalize(contact); err != nil {
		panic(err)
	}

	// Results after normalization:
	// - contact.PhoneRU: "+79991234567" (normalized with default RU region)
	// - contact.PhoneUS: "+12125551234" (normalized with US region)
	// - contact.PhoneDE: "+493012345678" (normalized with DE region)
	// - contact.MixedField: "+442079460958" (trimmed then normalized with GB region)
}
