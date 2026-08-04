// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqlbase holds the AST walk shared by the SQL filter translators.
//
// The ClickHouse, MariaDB and PostgreSQL translators differ in four places — how an identifier is quoted,
// how a bind marker is spelled, how the string predicates are expressed, and how a literal is rendered
// inline. Everything else is identical: the traversal, the allow-list and field-type checks, depth
// accounting, argument collection, IN lists, null handling and the comparison operators. That shared part
// lives in Translator; the four differences are supplied by a Dialect.
//
// # Usage
//
// A public translator package declares a stateless dialect and wraps the walker:
//
//	type Translator struct {
//	    *sqlbase.Translator
//	}
//
//	func NewTranslator(opts ...filter.TranslatorOption) (*Translator, error) {
//	    base, err := sqlbase.New(dialect{}, opts...)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &Translator{Translator: base}, nil
//	}
//
// Translate and TranslateInline are promoted from the embedded walker, so the public packages carry no
// translation logic of their own — only their dialect and its documentation.
//
// # Identifiers
//
// ValidateIdent, QuoteWhole and QuoteQualified implement the identifier policy every dialect needs. The
// validation is stricter than quoting requires on purpose: the column position is the one part of a
// generated clause that no placeholder can cover, so a name that is not a plain dot-separated identifier
// is rejected outright rather than escaped. Because the dialect's quote character cannot survive that
// check, the quoters need no escaping at all.
//
// The dot policy is the dialect's to choose. QuoteQualified produces the standard SQL "table"."column"
// used by MariaDB and PostgreSQL; QuoteWhole produces the single quoted name ClickHouse needs, where
// "address.city" is one Nested column rather than two identifiers.
package sqlbase
