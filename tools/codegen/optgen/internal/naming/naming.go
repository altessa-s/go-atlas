// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package naming

import (
	"strings"
	"unicode"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// initialisms maps a lower-cased word to its canonical all-caps form.
//
// The list is golint's commonInitialisms plus DB, which the standard library
// itself spells that way (database/sql.DB). Words absent here are left in
// MixedCaps, which is why gRPC is not listed: the repo spells it Grpc
// throughout (config.Grpc, DefaultGrpc), and a lone WithGRPCOptions would be
// less consistent, not more.
var initialisms = coremaps.NewImmutableMap(map[string]string{
	"acl":   "ACL",
	"api":   "API",
	"ascii": "ASCII",
	"cpu":   "CPU",
	"css":   "CSS",
	"db":    "DB",
	"dns":   "DNS",
	"eof":   "EOF",
	"guid":  "GUID",
	"html":  "HTML",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"ip":    "IP",
	"json":  "JSON",
	"lhs":   "LHS",
	"qps":   "QPS",
	"ram":   "RAM",
	"rhs":   "RHS",
	"rpc":   "RPC",
	"sla":   "SLA",
	"smtp":  "SMTP",
	"sql":   "SQL",
	"ssh":   "SSH",
	"tcp":   "TCP",
	"tls":   "TLS",
	"ttl":   "TTL",
	"udp":   "UDP",
	"ui":    "UI",
	"uid":   "UID",
	"uuid":  "UUID",
	"uri":   "URI",
	"url":   "URL",
	"utf8":  "UTF8",
	"vm":    "VM",
	"xml":   "XML",
	"xmpp":  "XMPP",
	"xsrf":  "XSRF",
	"xss":   "XSS",
})

// OptionName derives the exported option-name suffix from a struct field name:
// the name is capitalized and every word that is a known initialism is
// upper-cased, so a `ttl` field yields WithTTL rather than WithTtl.
//
// Callers that need a different spelling override it with the `opt` tag —
// `opt:"Ttl"` wins over this function.
//
//	OptionName("ttl")           == "TTL"
//	OptionName("httpClient")    == "HTTPClient"
//	OptionName("entityId")      == "EntityID"
//	OptionName("maxIdleTime")   == "MaxIdleTime"
func OptionName(fieldName string) string {
	if fieldName == "" {
		return fieldName
	}

	var b strings.Builder
	b.Grow(len(fieldName))

	for _, word := range splitWords(fieldName) {
		if upper, ok := initialisms.Get(strings.ToLower(word)); ok {
			b.WriteString(upper)
			continue
		}
		b.WriteString(capitalizeFirst(word))
	}

	return b.String()
}

// splitWords splits a camelCase or MixedCaps identifier into its words.
//
// Two boundaries matter: lower-to-upper ("maxIdle" -> "max", "Idle") and the
// tail of an all-caps run that starts the next word ("HTTPClient" -> "HTTP",
// "Client"). Without the second, an already-correct name would be re-split
// wrongly and lose its capitalization on a round trip.
func splitWords(s string) []string {
	runes := []rune(s)

	var (
		words []string
		start int
	)

	for i := 1; i < len(runes); i++ {
		switch {
		case unicode.IsUpper(runes[i]) && !unicode.IsUpper(runes[i-1]):
		case unicode.IsUpper(runes[i]) && unicode.IsUpper(runes[i-1]) &&
			i+1 < len(runes) && unicode.IsLower(runes[i+1]):
		default:
			continue
		}

		words = append(words, string(runes[start:i]))
		start = i
	}

	return append(words, string(runes[start:]))
}

// capitalizeFirst returns the string with its first letter capitalized.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}

	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])

	return string(r)
}
