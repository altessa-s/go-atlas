// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Webhook signature schemes for [WebhookSignature.Scheme].
const (
	WebhookSchemeGitHub = "github"
	WebhookSchemeStripe = "stripe"
)

// DefaultWebhookTolerance is the default replay window for a timestamped scheme,
// matching security/hmacsign's own default.
const DefaultWebhookTolerance = 5 * time.Minute

// WebhookSignature configures HMAC signing and verification of webhook request
// bodies (security/hmacsign). It drives the hmacsign factory, which builds a
// Verifier (to authenticate inbound webhooks) or a Signer (to sign outbound
// ones) for the selected provider scheme.
type WebhookSignature struct {
	// Scheme selects the provider wire format: "github" or "stripe".
	Scheme string `yaml:"scheme"`

	// Secret is the shared webhook secret. It is a Secret so it can be sourced
	// from a file/env/vault reference.
	Secret Secret `yaml:"secret"`

	// AdditionalSecrets are extra secrets the verifier also accepts, enabling
	// zero-downtime secret rotation. They are ignored when signing.
	AdditionalSecrets []Secret `yaml:"additionalSecrets"`

	// Tolerance is the replay window for a timestamped scheme (Stripe). Zero
	// disables the timestamp check.
	Tolerance time.Duration `yaml:"tolerance" default:"5m"`
}

// DefaultWebhookSignature returns a WebhookSignature with default values. Scheme
// and Secret are left empty since they are deployment-specific.
func DefaultWebhookSignature() WebhookSignature {
	return WebhookSignature{
		Tolerance: DefaultWebhookTolerance,
	}
}

// Validate performs validation on the WebhookSignature configuration. Scheme
// must be a supported value, Secret is required, and Tolerance must not be
// negative.
func (c *WebhookSignature) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Scheme, validation.Required,
			validation.In(WebhookSchemeGitHub, WebhookSchemeStripe)),
		validation.Field(&c.Secret, validation.Required),
		validation.Field(&c.Tolerance, validation.Min(time.Duration(0))),
	)
}
