// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
)

// extractClaims extracts all standard OIDC claims from the raw claims map
// and returns a structured Claims object with typed fields.
func extractClaims(raw map[string]any) *oidc.Claims {
	return &oidc.Claims{
		Subject:           stringClaim(raw, "sub"),
		PreferredUsername: stringClaim(raw, "preferred_username"),
		Email:             stringClaim(raw, "email"),
		Issuer:            stringClaim(raw, "iss"),
		Audience:          audienceClaim(raw, "aud"),
		Scopes:            scopesClaim(raw),
		ExpiresAt:         unixTimeClaim(raw, "exp"),
		IssuedAt:          unixTimeClaim(raw, "iat"),
		NotBefore:         unixTimeClaim(raw, "nbf"),
		FamilyName:        stringClaim(raw, "family_name"),
		Name:              stringClaim(raw, "name"),
		GivenName:         stringClaim(raw, "given_name"),
		RawClaims:         raw,
	}
}

// stringClaim extracts a string value from the claims map for the given key.
// It handles string values and types implementing fmt.Stringer interface.
// Returns empty string if the key is not found or value cannot be converted.
func stringClaim(claims map[string]any, key string) string {
	convertString := func(value any) string {
		switch typed := value.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
		return ""
	}

	if value, ok := claims[key]; ok {
		if str := convertString(value); str != "" {
			return str
		}
	}
	return ""
}

// audienceClaim extracts the audience claim which can be a single string or array of strings.
// Returns nil if the key is not found or the value is empty.
// The audience claim identifies the recipients that the JWT is intended for.
func audienceClaim(claims map[string]any, key string) []string {
	value, ok := claims[key]
	if !ok {
		return nil
	}

	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []string:
		if len(typed) == 0 {
			return nil
		}
		// Return as-is - Claims are typically read-only after extraction
		return typed
	case []any:
		// Single-pass conversion without intermediate allocations
		result := make([]string, 0, len(typed))
		for _, val := range typed {
			if str, ok := val.(string); ok {
				result = append(result, str)
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	}
	return nil
}

// scopesClaim extracts OAuth 2.0 scopes from the "scopes" claim.
// Supports three formats: space-separated string, string array, or mixed array.
// Empty scopes are filtered out and the result is sorted alphabetically.
// Returns nil if the key is not found or all scopes are empty.
func scopesClaim(claims map[string]any) []string {
	value, ok := claims["scopes"]
	if !ok {
		return nil
	}

	var scopes []string

	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		// strings.Fields already filters out empty strings
		scopes = strings.Fields(typed)
	case []string:
		// Single-pass filter for non-empty strings
		scopes = make([]string, 0, len(typed))
		for _, s := range typed {
			if s != "" {
				scopes = append(scopes, s)
			}
		}
	case []any:
		// Single-pass conversion and filter for string values
		scopes = make([]string, 0, len(typed))
		for _, val := range typed {
			if str, ok := val.(string); ok && str != "" {
				scopes = append(scopes, str)
			}
		}
	}

	if len(scopes) == 0 {
		return nil
	}

	slices.Sort(scopes)
	return scopes
}

// unixTimeClaim extracts a Unix timestamp from the claims and converts it to time.Time.
// Handles multiple numeric types: float64, int, int64, and json.Number.
// Returns zero time if the key is not found, value is invalid, or less than/equal to zero.
// All timestamps are converted to UTC.
func unixTimeClaim(claims map[string]any, key string) time.Time {
	value, ok := claims[key]
	if !ok {
		return time.Time{}
	}

	switch typed := value.(type) {
	case float64:
		return time.Unix(int64(typed), 0).UTC()
	case int:
		return time.Unix(int64(typed), 0).UTC()
	case int64:
		return time.Unix(typed, 0).UTC()
	case json.Number:
		v, err := typed.Int64()
		if err != nil || v <= 0 {
			return time.Time{}
		}
		return time.Unix(v, 0).UTC()
	}
	return time.Time{}
}
