// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"testing"
)

func FuzzExtractor_Extract(f *testing.F) {
	f.Add("8.8.8.8", "1.2.3.4")
	f.Add("10.0.0.1", "203.0.114.1")
	f.Add("192.168.1.1", "invalid-ip")
	f.Add("127.0.0.1", "")

	ext, err := NewExtractor(WithCacheDisabled())
	if err != nil {
		f.Fatalf("NewExtractor() error = %v", err)
	}

	f.Fuzz(func(t *testing.T, peerIPStr, headerIP string) {
		peerIP, err := netip.ParseAddr(peerIPStr)
		if err != nil {
			return
		}

		headers := &fuzzHeaders{data: map[string][]string{
			HeaderXForwardedFor: {headerIP},
		}}

		// Should not panic
		ext.Extract(t.Context(), peerIP, headers)
	})
}

type fuzzHeaders struct {
	data map[string][]string
}

func (m *fuzzHeaders) GetHeader(name string) []string {
	return m.data[name]
}
