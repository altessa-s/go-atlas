// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// FuzzNetworkMatchersLookOnlyInsideAURLError pins the shape these classifiers
// require before they will say yes.
//
// They are string matchers over an error message, which is the fragile kind:
// the message comes from net/http and, in a proxy or a retry wrapper, can carry
// text a remote peer influenced. Both are documented to answer only for a
// *url.Error, so a message that merely mentions redirects — in an unrelated
// error, or in the URL itself — must not be enough. Otherwise a retry loop
// classifies a failure by a substring an attacker put in a hostname.
func FuzzNetworkMatchersLookOnlyInsideAURLError(f *testing.F) {
	f.Add("stopped after 10 redirects", true)
	f.Add("unsupported protocol scheme \"ftp\"", true)
	f.Add("random error string", true)
	f.Add("stopped after 10 redirects", false)
	f.Add("", true)

	f.Fuzz(func(t *testing.T, message string, wrapped bool) {
		plain := errors.New(message)

		if !wrapped {
			// Not a *url.Error: neither matcher may claim it, whatever it says.
			require.False(t, coreerrs.IsResourceRedirects(plain),
				"a plain error was classified as a redirect failure: %q", message)
			require.False(t, coreerrs.IsUnsupportedProtocolScheme(plain),
				"a plain error was classified as a scheme failure: %q", message)
			return
		}

		// The URL field is caller-controlled — in a proxy it carries a target a
		// client chose — while the verdict is documented to come from the
		// wrapped error's message. Two errors differing only in URL must
		// therefore classify identically; restating the regex here instead
		// would just be guessing at it.
		withURL := &url.Error{Op: "Get", URL: "http://" + message, Err: plain}
		withoutURL := &url.Error{Op: "Get", URL: "http://example.com", Err: plain}

		require.Equal(t, coreerrs.IsResourceRedirects(withoutURL), coreerrs.IsResourceRedirects(withURL),
			"the redirect verdict changed with the URL field: %q", message)
		require.Equal(t, coreerrs.IsUnsupportedProtocolScheme(withoutURL), coreerrs.IsUnsupportedProtocolScheme(withURL),
			"the scheme verdict changed with the URL field: %q", message)
	})
}

// FuzzWrappingPreservesClassification pins that the matchers see through the
// repo's own wrappers.
//
// Every call site wraps errors with coreerrs.Wrap on the way up, so a
// classifier that only inspected the outermost error would stop recognizing a
// redirect or a canceled context the moment someone added context to the
// message — and the retry policy built on it would silently change behavior.
func FuzzWrappingPreservesClassification(f *testing.F) {
	f.Add("stopped after 10 redirects", "fetching config", uint8(3))
	f.Add("unsupported protocol scheme", "", uint8(0))
	f.Add("plain", "op", uint8(1))

	f.Fuzz(func(t *testing.T, message, context string, depth uint8) {
		inner := &url.Error{Op: "Get", URL: "http://example.com", Err: errors.New(message)}

		wantRedirects := coreerrs.IsResourceRedirects(inner)
		wantScheme := coreerrs.IsUnsupportedProtocolScheme(inner)

		var err error = inner
		for range int(depth) % 8 {
			err = fmt.Errorf("%s: %w", context, err)
		}

		require.Equal(t, wantRedirects, coreerrs.IsResourceRedirects(err),
			"wrapping %d deep changed the redirect verdict for %q", depth%8, message)
		require.Equal(t, wantScheme, coreerrs.IsUnsupportedProtocolScheme(err),
			"wrapping %d deep changed the scheme verdict for %q", depth%8, message)
	})
}

// FuzzContextClassifiersAgreeWithErrorsIs pins the context helpers against the
// standard library they wrap.
//
// These decide whether a failed dispatch is transient — the outbox uses exactly
// this to avoid dead-lettering healthy events during a broker outage — so a
// divergence from errors.Is is a classification the rest of the codebase
// assumes cannot happen.
func FuzzContextClassifiersAgreeWithErrorsIs(f *testing.F) {
	f.Add(uint8(0), "wrapped", uint8(2))
	f.Add(uint8(1), "", uint8(0))
	f.Add(uint8(2), "op", uint8(5))

	f.Fuzz(func(t *testing.T, kind uint8, message string, depth uint8) {
		bases := []error{context.Canceled, context.DeadlineExceeded, errors.New(message), nil}
		base := bases[int(kind)%len(bases)]

		err := base
		for range int(depth) % 8 {
			if err == nil {
				break
			}
			err = fmt.Errorf("%s: %w", message, err)
		}

		require.Equal(t, err != nil && errors.Is(err, context.Canceled), coreerrs.IsContextCanceled(err))
		require.Equal(t, err != nil && errors.Is(err, context.DeadlineExceeded), coreerrs.IsContextDeadlineExceeded(err))
		require.Equal(t,
			coreerrs.IsContextCanceled(err) || coreerrs.IsContextDeadlineExceeded(err),
			coreerrs.IsContextCanceledOrDeadlineExceeded(err),
			"the combined classifier is not the disjunction of its parts")
	})
}
