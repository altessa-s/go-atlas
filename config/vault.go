// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"strings"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Vault configuration.
const (
	defaultVaultAddress    = "http://localhost:8200"
	defaultVaultAuthMethod = VaultAuthMethodToken
)

// VaultAuthMethod represents the authentication method used to authenticate with Vault.
type VaultAuthMethod string

const (
	// VaultAuthMethodToken uses a Vault token for authentication.
	VaultAuthMethodToken VaultAuthMethod = "token"
	// VaultAuthMethodAppRole uses AppRole authentication for machine-to-machine authentication.
	VaultAuthMethodAppRole VaultAuthMethod = "approle"
	// VaultAuthMethodUserPass uses username/password authentication for user authentication.
	VaultAuthMethodUserPass VaultAuthMethod = "userpass"
)

// VaultAuthApprole represents the AppRole authentication method.
type VaultAuthApprole struct {
	RoleId    string  `yaml:"roleId"`
	SecretId  Secret  `yaml:"secretId"`
	MountPath *string `yaml:"mountPath" default:"-"`
}

// VaultAuthUserpass represents the Userpass authentication method.
type VaultAuthUserpass struct {
	Username  string  `yaml:"username"`
	Password  Secret  `yaml:"password"`
	MountPath *string `yaml:"mountPath" default:"-"`
}

// VaultAuth configures authentication settings for connecting to Vault.
// Only one authentication method should be configured based on the Method field.
type VaultAuth struct {
	Method   VaultAuthMethod    `yaml:"method" default:"token"`
	Token    *Secret            `yaml:"token" default:"-"`
	Approle  *VaultAuthApprole  `yaml:"approle" default:"-"`
	Userpass *VaultAuthUserpass `yaml:"userpass" default:"-"`
}

// Validate checks that the VaultAuth configuration is valid.
// It ensures that the authentication method is supported and that the corresponding
// authentication configuration is provided.
func (va *VaultAuth) Validate() error {
	return ValidateStruct(va,
		validation.Field(&va.Method, ozzo_rules.OneOf(
			VaultAuthMethodToken,
			VaultAuthMethodAppRole,
			VaultAuthMethodUserPass)),
		validation.Field(&va.Token, validation.When(va.Method == VaultAuthMethodToken, validation.Required)),
		validation.Field(&va.Approle, validation.When(va.Method == VaultAuthMethodAppRole, validation.Required)),
		validation.Field(&va.Userpass, validation.When(va.Method == VaultAuthMethodUserPass, validation.Required)),
	)
}

// Vault represents the configuration for connecting to a HashiCorp Vault server.
// It includes authentication settings, TLS configuration, and server address.
//
// Example:
//
//	vault := &config.Vault{
//		Address: "https://vault.example.com:8200",
//		Auth:    &config.VaultAuth{Method: config.VaultAuthMethodToken, Token: ptr.Wrap("s.secret")},
//	}
type Vault struct {
	Auth *VaultAuth `yaml:"auth" default:"-"`

	// TLS indicates we should use a secure connection while talking to Vault.
	TLS *TlsClient `yaml:"tls" default:"-"`

	// Host is the address of the Vault server. It may be an IP or FQDN.
	Address string `yaml:"address" default:"http://localhost:8200"`
}

// Normalize prepares and normalizes the Vault configuration parameters.
// It automatically adds the appropriate URL scheme (http:// or https://) to the address
// if not already present, based on whether TLS is configured.
func (v *Vault) Normalize() {
	if !strings.Contains(v.Address, "://") {
		scheme := "http://"
		if v.TLS != nil {
			scheme = "https://"
		}
		v.Address = scheme + v.Address
	}
}

// DefaultVault returns a Vault configuration with default values.
// Note: Auth is left as nil since it is a required field that must be configured.
func DefaultVault() Vault {
	return Vault{
		Address: defaultVaultAddress,
	}
}

// DefaultVaultAuth returns a VaultAuth configuration with default values.
// Note: Token is left as nil since it is required when using token auth method.
func DefaultVaultAuth() VaultAuth {
	return VaultAuth{
		Method: defaultVaultAuthMethod,
	}
}

// Validate checks if the Vault configuration is valid.
// It validates the server address as a proper URL and ensures that authentication
// configuration is provided and valid.
func (v *Vault) Validate() error {
	return ValidateStruct(v,
		validation.Field(&v.Address, is.URL),
		validation.Field(&v.TLS, validation.When(v.TLS != nil, validation.Required)),
		validation.Field(&v.Auth, validation.Required),
	)
}
