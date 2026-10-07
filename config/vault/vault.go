// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vaultconfig

import (
	"strings"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Vault configuration.
const (
	defaultVaultAddress    = "http://localhost:8200"
	defaultVaultAuthMethod = AuthMethodToken
)

// AuthMethod represents the authentication method used to authenticate with Vault.
type AuthMethod string

const (
	// AuthMethodToken uses a Vault token for authentication.
	AuthMethodToken AuthMethod = "token"
	// AuthMethodAppRole uses AppRole authentication for machine-to-machine authentication.
	AuthMethodAppRole AuthMethod = "approle"
	// AuthMethodUserPass uses username/password authentication for user authentication.
	AuthMethodUserPass AuthMethod = "userpass"
)

// AuthAppRole represents the AppRole authentication method.
type AuthAppRole struct {
	RoleId    string                  `yaml:"roleID"`
	SecretId  redacted.RedactedString `yaml:"secretID"`
	MountPath *string                 `yaml:"mountPath" default:"-"`
}

// AuthUserPass represents the Userpass authentication method.
type AuthUserPass struct {
	Username  string                  `yaml:"username"`
	Password  redacted.RedactedString `yaml:"password"`
	MountPath *string                 `yaml:"mountPath" default:"-"`
}

// Auth configures authentication settings for connecting to Vault.
// Only one authentication method should be configured based on the Method field.
type Auth struct {
	Method   AuthMethod               `yaml:"method" default:"token"`
	Token    *redacted.RedactedString `yaml:"token" default:"-"`
	Approle  *AuthAppRole             `yaml:"approle" default:"-"`
	Userpass *AuthUserPass            `yaml:"userpass" default:"-"`
}

// Validate checks that the Auth configuration is valid.
// It ensures that the authentication method is supported and that the corresponding
// authentication configuration is provided.
func (va *Auth) Validate() error {
	return validationconfig.ValidateStruct(va,
		validation.Field(&va.Method, ozzo_rules.OneOf(
			AuthMethodToken,
			AuthMethodAppRole,
			AuthMethodUserPass)),
		validation.Field(&va.Token, validation.When(va.Method == AuthMethodToken, validation.Required)),
		validation.Field(&va.Approle, validation.When(va.Method == AuthMethodAppRole, validation.Required)),
		validation.Field(&va.Userpass, validation.When(va.Method == AuthMethodUserPass, validation.Required)),
	)
}

// Config represents the configuration for connecting to a HashiCorp Vault server.
// It includes authentication settings, TLS configuration, and server address.
//
// Example:
//
//	vault := &vaultconfig.Config{
//		Address: "https://vault.example.com:8200",
//		Auth:    &vaultconfig.Auth{Method: vaultconfig.AuthMethodToken, Token: new("s.secret")},
//	}
type Config struct {
	Auth *Auth `yaml:"auth" default:"-"`

	// TLS indicates we should use a secure connection while talking to Vault.
	TLS *tlsconfig.Client `yaml:"tls" default:"-"`

	// Host is the address of the Vault server. It may be an IP or FQDN.
	Address string `yaml:"address" default:"http://localhost:8200"`
}

// Normalize prepares and normalizes the Vault configuration parameters.
// It automatically adds the appropriate URL scheme (http:// or https://) to the address
// if not already present, based on whether TLS is configured.
func (v *Config) Normalize() {
	if !strings.Contains(v.Address, "://") {
		scheme := "http://"
		if v.TLS != nil {
			scheme = "https://"
		}
		v.Address = scheme + v.Address
	}
}

// Default returns a Vault configuration with default values.
// Note: Auth is left as nil since it is a required field that must be configured.
func Default() Config {
	return Config{
		Address: defaultVaultAddress,
	}
}

// DefaultAuth returns a Auth configuration with default values.
// Note: Token is left as nil since it is required when using token auth method.
func DefaultAuth() Auth {
	return Auth{
		Method: defaultVaultAuthMethod,
	}
}

// Validate checks if the Vault configuration is valid.
// It validates the server address as a proper URL and ensures that authentication
// configuration is provided and valid.
func (v *Config) Validate() error {
	return validationconfig.ValidateStruct(v,
		validation.Field(&v.Address, is.URL),
		validation.Field(&v.TLS, validation.When(v.TLS != nil, validation.Required)),
		validation.Field(&v.Auth, validation.Required),
	)
}
