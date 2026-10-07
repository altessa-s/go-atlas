// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package webhookconfig

import (
	"time"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Webhook signature schemes for [Signature.Scheme].
const (
	SchemeGitHub = "github"
	SchemeStripe = "stripe"
)

// DefaultTolerance is the default replay window for a timestamped scheme,
// matching security/hmacsign's own default.
const DefaultTolerance = 5 * time.Minute

// Signature configures HMAC signing and verification of webhook request
// bodies (security/hmacsign). It drives the hmacsign factory, which builds a
// Verifier (to authenticate inbound webhooks) or a Signer (to sign outbound
// ones) for the selected provider scheme.
type Signature struct {
	// Scheme selects the provider wire format: "github" or "stripe".
	Scheme string `yaml:"scheme"`

	// Secret is the shared webhook secret. It is a Secret so it can be sourced
	// from a file/env/vault reference.
	Secret redacted.RedactedString `yaml:"secret"`

	// AdditionalSecrets are extra secrets the verifier also accepts, enabling
	// zero-downtime secret rotation. They are ignored when signing.
	AdditionalSecrets []redacted.RedactedString `yaml:"additionalSecrets"`

	// Tolerance is the replay window for a timestamped scheme (Stripe). Zero
	// disables the timestamp check.
	Tolerance time.Duration `yaml:"tolerance" default:"5m"`
}

// DefaultSignature returns a Signature with default values. Scheme
// and Secret are left empty since they are deployment-specific.
func DefaultSignature() Signature {
	return Signature{
		Tolerance: DefaultTolerance,
	}
}

// Validate performs validation on the Signature configuration. Scheme
// must be a supported value, Secret is required, and Tolerance must not be
// negative.
func (c *Signature) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Scheme, validation.Required,
			validation.In(SchemeGitHub, SchemeStripe)),
		validation.Field(&c.Secret, validation.Required),
		validation.Field(&c.Tolerance, validation.Min(time.Duration(0))),
	)
}
