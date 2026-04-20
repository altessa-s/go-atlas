// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package topics

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// MaxSubjectLength caps the length of a generated NATS subject. NATS rejects
// subjects longer than 255 bytes; [Topic.WithValidation] enforces this limit.
const MaxSubjectLength = 255

// Wildcard is the NATS single-token wildcard. It matches exactly one token
// (segment between dots) and is preserved literally when used as a macro
// value via [Topic.With] or [Topic.WithValidation].
const Wildcard = "*"

// FullWildcard is the NATS multi-token wildcard. It matches zero or more
// trailing tokens and must be the last token of the subject. It is preserved
// literally when used as a macro value via [Topic.With] or
// [Topic.WithValidation].
const FullWildcard = ">"

// Sentinel errors returned by [Topic.WithValidation].
var (
	ErrSubjectTooLong          = errors.New("generated subject exceeds maximum length of 255 characters")
	ErrInvalidWildcardPosition = errors.New("full wildcard ('>') must be the last token in the subject")
	ErrEmptyMacroValue         = errors.New("macro value cannot be empty")
	ErrInvalidCharacters       = errors.New("value contains characters that cannot be used in NATS subjects")
)

// macroCache stores macros parsed from each [Topic] so subsequent extractions
// avoid reparsing the template. Keys are Topic values (immutable string
// constants), so the cache is bounded by the number of distinct topic
// templates declared in the program.
var macroCache sync.Map // map[Topic]map[TopicKey]struct{}

// Topic is a NATS subject template that may contain macro placeholders written
// in curly braces (e.g. "events.{TENANT}"). Macros are substituted with
// concrete values via [Topic.With] or [Topic.WithValidation]; the original
// template value is never modified.
//
// Topic values are usually declared as package-level string constants:
//
//	const tenantEvents topics.Topic = "events.{TENANT}"
//
// Macro names must consist of uppercase letters, digits, and underscores
// (validated by [Topic.Validate]). All macros declared in the template must be
// supplied when calling [Topic.With] or [Topic.WithValidation].
type Topic string

// With substitutes macros in t with values from str (key, value, key, value …)
// and returns the resulting NATS subject. Macro values are sanitized (".", "*"
// and ">" become "_") except for the literal [Wildcard] and [FullWildcard]
// constants, which are preserved.
//
// With panics on programmer error - an odd argument count, an unknown macro
// name, a missing required macro, or arguments supplied for a topic without
// macros. The panic value is a string describing the failure. For untrusted
// input, use [Topic.WithValidation], which returns an error and additionally
// enforces [MaxSubjectLength] and wildcard positioning.
//
// Example:
//
//	const tenantEvents topics.Topic = "events.{TENANT}"
//	subject := tenantEvents.With("TENANT", "acme")
//	// subject == "events.acme"
func (t Topic) With(str ...string) string {
	subject, err := t.substitute(str, false)
	if err != nil {
		panic(err.Error())
	}
	return subject
}

// WithValidation behaves like [Topic.With] but returns an error instead of
// panicking and additionally enforces [MaxSubjectLength], wildcard positioning,
// and macro value character restrictions. Use it whenever any input is
// untrusted.
//
// Returned errors wrap one of the package sentinels ([ErrSubjectTooLong],
// [ErrInvalidWildcardPosition], [ErrEmptyMacroValue], [ErrInvalidCharacters])
// where applicable, so callers may use [errors.Is] for programmatic handling.
func (t Topic) WithValidation(str ...string) (string, error) {
	return t.substitute(str, true)
}

// substitute is the shared substitution path for With and WithValidation.
// When validate is true, macro values are validated before substitution and
// the resulting subject is checked against MaxSubjectLength and wildcard
// positioning rules.
func (t Topic) substitute(str []string, validate bool) (string, error) {
	macros := t.extractMacros()
	switch {
	case len(macros) == 0 && len(str) > 0:
		return "", fmt.Errorf("invalid number of arguments: topic has no macros but %d arguments provided", len(str))
	case len(macros) > 0 && (len(str) == 0 || len(str)%2 != 0 || len(macros) != len(str)/2):
		return "", fmt.Errorf("invalid number of arguments: expected %d key-value pairs, got %d arguments", len(macros), len(str))
	}

	subject := string(t)
	provided := make(map[TopicKey]struct{}, len(macros))
	for i := 0; i < len(str); i += 2 {
		key := TopicKey(str[i])
		if _, ok := macros[key]; !ok {
			return "", fmt.Errorf("unknown macro '%s': not found in topic template", str[i])
		}
		if validate {
			if err := validateMacroValue(str[i+1]); err != nil {
				return "", coreerrs.Wrapf(err, "invalid value for macro '%s'", str[i])
			}
		}
		provided[key] = struct{}{}
		subject = strings.ReplaceAll(subject, key.template(), sanitizeValue(str[i+1]))
	}

	for macro := range macros {
		if _, ok := provided[macro]; !ok {
			return "", fmt.Errorf("missing macro '%s': required by topic template", macro)
		}
	}

	if !validate {
		return subject, nil
	}
	if len(subject) > MaxSubjectLength {
		return "", fmt.Errorf("%w: got %d characters", ErrSubjectTooLong, len(subject))
	}
	if err := validateWildcardPosition(subject); err != nil {
		return "", err
	}
	return subject, nil
}

// AcceptsMacros reports whether t declares all keys. With no keys, it reports
// whether t declares any macros at all. Useful for compatibility checks before
// calling [Topic.With] or [Topic.WithValidation].
func (t Topic) AcceptsMacros(keys ...TopicKey) bool {
	macros := t.extractMacros()
	if len(keys) == 0 {
		return len(macros) > 0
	}
	for _, key := range keys {
		if _, ok := macros[key]; !ok {
			return false
		}
	}
	return true
}

// RequiredMacros returns the macro keys declared by t, sorted alphabetically.
// The result is a fresh slice that the caller may mutate freely.
func (t Topic) RequiredMacros() []TopicKey {
	macros := t.extractMacros()
	keys := make([]TopicKey, 0, len(macros))
	for k := range macros {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Validate verifies the structural integrity of the topic template:
// non-empty, within [MaxSubjectLength], properly paired braces, and macro
// names containing only uppercase ASCII letters, digits, and underscores.
// It does not check whether the template's substituted form is a legal NATS
// subject - that is the job of [Topic.WithValidation].
func (t Topic) Validate() error {
	s := string(t)
	if s == "" {
		return errors.New("topic template cannot be empty")
	}
	if len(s) > MaxSubjectLength {
		return fmt.Errorf("topic template exceeds maximum length: %d > %d", len(s), MaxSubjectLength)
	}

	openBrace := -1
	for i, r := range s {
		switch r {
		case '{':
			if openBrace != -1 {
				return fmt.Errorf("nested or unclosed macro at position %d", i)
			}
			if i > 0 && s[i-1] != '.' {
				return fmt.Errorf("macro at position %d must be preceded by '.'", i)
			}
			openBrace = i
		case '}':
			if openBrace == -1 {
				return fmt.Errorf("closing brace without opening brace at position %d", i)
			}
			name := s[openBrace+1 : i]
			if name == "" {
				return fmt.Errorf("empty macro name at position %d", openBrace)
			}
			for _, c := range name {
				if (c < 'A' || c > 'Z') && c != '_' && (c < '0' || c > '9') {
					return fmt.Errorf("invalid character %q in macro name %q", c, name)
				}
			}
			if i < len(s)-1 && s[i+1] != '.' {
				return fmt.Errorf("macro at position %d must be followed by '.'", openBrace)
			}
			openBrace = -1
		}
	}
	if openBrace != -1 {
		return fmt.Errorf("unclosed macro starting at position %d", openBrace)
	}
	return nil
}

// extractMacros returns the cached set of macro keys declared in t. The
// returned map MUST NOT be mutated by callers; treat it as read-only.
func (t Topic) extractMacros() map[TopicKey]struct{} {
	if cached, ok := macroCache.Load(t); ok {
		if m, ok := cached.(map[TopicKey]struct{}); ok {
			return m
		}
	}

	s := string(t)
	macros := make(map[TopicKey]struct{})
	for i := 0; i < len(s); {
		open := strings.IndexByte(s[i:], '{')
		if open < 0 {
			break
		}
		open += i
		end := strings.IndexByte(s[open+1:], '}')
		if end < 0 {
			break
		}
		end += open + 1
		if name := s[open+1 : end]; name != "" {
			macros[TopicKey(name)] = struct{}{}
		}
		i = end + 1
	}

	macroCache.Store(t, macros)
	return macros
}

// validateMacroValue rejects empty values and values containing characters
// that cannot be sanitized to a valid NATS subject token. [Wildcard] and
// [FullWildcard] are accepted unchanged.
func validateMacroValue(value string) error {
	if value == "" {
		return ErrEmptyMacroValue
	}
	if value == Wildcard || value == FullWildcard {
		return nil
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_',
			r == '.', r == '*', r == '>':
			// allowed - dots/asterisks/greater-than are sanitized below
		default:
			return ErrInvalidCharacters
		}
	}
	return nil
}

// sanitizeValue replaces ".", "*" and ">" with "_" so the value can safely be
// embedded as a single NATS subject token. [Wildcard] and [FullWildcard] are
// returned unchanged so subscription patterns remain intact.
func sanitizeValue(val string) string {
	if val == Wildcard || val == FullWildcard {
		return val
	}
	if !strings.ContainsAny(val, ".*>") {
		return val
	}
	var b strings.Builder
	b.Grow(len(val))
	for _, r := range val {
		switch r {
		case '.', '*', '>':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// validateWildcardPosition ensures FullWildcard, when present, is the final
// token of the subject and is preceded by a dot (or is the entire subject).
func validateWildcardPosition(subject string) error {
	pos := strings.Index(subject, FullWildcard)
	if pos == -1 {
		return nil
	}
	if pos != len(subject)-1 {
		return ErrInvalidWildcardPosition
	}
	if pos > 0 && subject[pos-1] != '.' {
		return ErrInvalidWildcardPosition
	}
	return nil
}
