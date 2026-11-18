// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewareconfig

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/altessa-s/go-atlas/config/internal/cfgerrors"
	"github.com/altessa-s/go-atlas/config/internal/validators"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	validationconfig "github.com/altessa-s/go-nomos/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GeoACLRule defines a geographic access control rule for one or more endpoints.
type GeoACLRule struct {
	// Endpoints is a list of exact endpoint names this rule applies to.
	Endpoints []string `yaml:"endpoints"`

	// Patterns is a list of regex patterns for endpoints this rule applies to.
	Patterns []string `yaml:"patterns"`

	// AllowContinents is a list of continent codes that are explicitly allowed.
	AllowContinents []string `yaml:"allowContinents"`

	// DenyContinents is a list of continent codes that are explicitly denied.
	DenyContinents []string `yaml:"denyContinents"`

	// AllowCountries is a list of ISO 3166-1 alpha-2 country codes that are explicitly allowed.
	AllowCountries []string `yaml:"allowCountries"`

	// DenyCountries is a list of ISO 3166-1 alpha-2 country codes that are explicitly denied.
	DenyCountries []string `yaml:"denyCountries"`

	// AllowRegions is a list of region codes (e.g. "US-CA") that are explicitly allowed.
	AllowRegions []string `yaml:"allowRegions"`

	// DenyRegions is a list of region codes (e.g. "US-TX") that are explicitly denied.
	DenyRegions []string `yaml:"denyRegions"`
}

// validContinentCodes is the set of valid 2-letter continent codes.
var validContinentCodes = coremaps.NewImmutableMap(map[string]struct{}{
	"AF": {}, "AN": {}, "AS": {}, "EU": {}, "NA": {}, "OC": {}, "SA": {},
})

// regionCodePattern matches XX-YY format for region codes.
var regionCodePattern = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,3}$`)

// ValidateGeoACLRule validates a single Geo ACL rule entry.
func ValidateGeoACLRule(value any) error {
	rule, ok := value.(GeoACLRule)
	if !ok {
		return nil
	}
	return validationconfig.ValidateStruct(&rule,
		validation.Field(&rule.Patterns, validation.Each(validation.Required, ozzo_rules.Regex())),
		validation.Field(&rule.AllowContinents, validation.Each(validation.By(validateContinentCode))),
		validation.Field(&rule.DenyContinents, validation.Each(validation.By(validateContinentCode))),
		validation.Field(&rule.AllowCountries, validation.Each(validation.By(validateCountryCode))),
		validation.Field(&rule.DenyCountries, validation.Each(validation.By(validateCountryCode))),
		validation.Field(&rule.AllowRegions, validation.Each(validation.By(validateRegionCode))),
		validation.Field(&rule.DenyRegions, validation.Each(validation.By(validateRegionCode))),
	)
}

// validateContinentCode validates a continent code is one of: AF, AN, AS, EU, NA, OC, SA.
func validateContinentCode(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if !validContinentCodes.Contains(s) {
		return fmt.Errorf("invalid continent code %q; must be one of: AF, AN, AS, EU, NA, OC, SA", s)
	}
	return nil
}

// validateCountryCode requires an uppercase ISO 3166-1 alpha-2 country code.
func validateCountryCode(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if s != strings.ToUpper(s) || ozzo_rules.CountryCode2().Validate(s) != nil {
		return fmt.Errorf("invalid country code %q; must be 2 uppercase letters (ISO 3166-1 alpha-2)", s)
	}
	return nil
}

// validateRegionCode validates a region code in XX-YY format.
func validateRegionCode(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if !regionCodePattern.MatchString(s) {
		return fmt.Errorf("invalid region code %q; must be in XX-YY format (e.g. US-CA)", s)
	}
	return nil
}

// IPACLRule defines an IP access control rule for one or more endpoints.
type IPACLRule struct {
	// Endpoints is a list of exact endpoint names this rule applies to.
	Endpoints []string `yaml:"endpoints"`

	// Patterns is a list of regex patterns for endpoints this rule applies to.
	Patterns []string `yaml:"patterns"`

	// Allowlist is a list of CIDR prefixes that are explicitly allowed.
	Allowlist []string `yaml:"allowlist"`

	// Denylist is a list of CIDR prefixes that are explicitly denied.
	Denylist []string `yaml:"denylist"`
}

// ValidateIPACLRule validates a single IP ACL rule entry.
func ValidateIPACLRule(value any) error {
	rule, ok := value.(IPACLRule)
	if !ok {
		return nil
	}
	return validationconfig.ValidateStruct(&rule,
		validation.Field(&rule.Patterns, validation.Each(validation.Required, ozzo_rules.Regex())),
		validation.Field(&rule.Allowlist, validation.Each(validation.By(ValidateIP))),
		validation.Field(&rule.Denylist, validation.Each(validation.By(ValidateIP))),
	)
}

// ValidateIP validates that a value is a CIDR prefix.
// This function ensures that trusted proxy and peer entries are properly formatted.
func ValidateIP(value any) error {
	str, ok := value.(string)
	if !ok {
		// Should not happen with struct validation
		return cfgerrors.Invalidf("invalid type for IP validation")
	}
	if err := validators.CIDRPrefix.Validate(str); err != nil {
		return cfgerrors.Invalidf("invalid IP: '%s'", str)
	}
	return nil
}
