// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hmacsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Scheme encodes a webhook provider's signature wire format: the header the
// signature lives in, the canonical message that is HMAC'd, and how that message
// is rendered into and parsed out of the header value. The digest algorithm and
// secret are owned by [Signer]/[Verifier], not the scheme.
//
// The built-in [GitHub] and [Stripe] schemes cover the two dominant styles.
// Implement this interface for a provider with a different grammar (for example
// Shopify's base64 body signature); a scheme is stateless and safe to share.
type Scheme interface {
	// HeaderName is the HTTP header the signature is carried in.
	HeaderName() string

	// Message returns the canonical bytes whose HMAC is the signature, for a body
	// signed at signedAt. A scheme that does not bind a timestamp ignores it.
	Message(signedAt time.Time, body []byte) []byte

	// FormatHeader renders the signature header value from the signing time and
	// the raw digest of [Scheme.Message]. A scheme that does not bind a timestamp
	// ignores signedAt.
	FormatHeader(signedAt time.Time, digest []byte) string

	// ParseHeader extracts, from a received header value, the signed timestamp
	// (the zero time when the scheme binds none — which turns off the replay
	// window) and the candidate raw digests to compare against. It returns
	// [ErrMalformedSignature] when the header cannot be parsed.
	ParseHeader(header string) (signedAt time.Time, digests [][]byte, err error)
}

// digest returns the HMAC-SHA256 of msg under secret.
func digest(secret, msg []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write(msg)
	return m.Sum(nil)
}

// GitHub returns the GitHub webhook signature scheme: header
// "X-Hub-Signature-256", value "sha256=<hex>", HMAC-SHA256 over the raw body.
func GitHub() Scheme { return githubScheme{} }

// Stripe returns the Stripe webhook signature scheme: header "Stripe-Signature",
// value "t=<unix>,v1=<hex>", HMAC-SHA256 over "<t>.<body>". The signed timestamp
// enables replay protection via [WithTolerance].
func Stripe() Scheme { return stripeScheme{} }

const (
	githubHeader = "X-Hub-Signature-256"
	githubPrefix = "sha256="
)

type githubScheme struct{}

func (githubScheme) HeaderName() string { return githubHeader }

func (githubScheme) Message(_ time.Time, body []byte) []byte { return body }

func (githubScheme) FormatHeader(_ time.Time, digest []byte) string {
	return githubPrefix + hex.EncodeToString(digest)
}

func (githubScheme) ParseHeader(header string) (time.Time, [][]byte, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(header), githubPrefix))
	if err != nil || len(raw) == 0 {
		return time.Time{}, nil, fmt.Errorf("%w: bad digest", ErrMalformedSignature)
	}
	return time.Time{}, [][]byte{raw}, nil
}

const stripeHeader = "Stripe-Signature"

type stripeScheme struct{}

func (stripeScheme) HeaderName() string { return stripeHeader }

func (stripeScheme) Message(signedAt time.Time, body []byte) []byte {
	return stripeMessage(signedAt.Unix(), body)
}

func (stripeScheme) FormatHeader(signedAt time.Time, digest []byte) string {
	return fmt.Sprintf("t=%d,v1=%s", signedAt.Unix(), hex.EncodeToString(digest))
}

func (stripeScheme) ParseHeader(header string) (time.Time, [][]byte, error) {
	var (
		tsUnix int64
		haveTS bool
		sigs   [][]byte
	)
	for part := range strings.SplitSeq(header, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return time.Time{}, nil, fmt.Errorf("%w: bad timestamp", ErrMalformedSignature)
			}
			tsUnix, haveTS = n, true
		case "v1":
			raw, err := hex.DecodeString(val)
			if err != nil {
				return time.Time{}, nil, fmt.Errorf("%w: bad v1 digest", ErrMalformedSignature)
			}
			sigs = append(sigs, raw)
		}
	}
	if !haveTS || len(sigs) == 0 {
		return time.Time{}, nil, fmt.Errorf("%w: missing t or v1", ErrMalformedSignature)
	}
	return time.Unix(tsUnix, 0), sigs, nil
}

// stripeMessage builds Stripe's signed payload: the Unix timestamp, a dot, then
// the raw body.
func stripeMessage(tsUnix int64, body []byte) []byte {
	msg := strconv.AppendInt(nil, tsUnix, 10)
	msg = append(msg, '.')
	return append(msg, body...)
}
