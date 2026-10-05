// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package regexanchor rewrites the end-of-text anchors of an RE2 pattern so
// that PCRE and ICU engines read them the way RE2 does.
//
// CEL's matches() is RE2: outside multi-line mode `$` matches only at the
// very end of the text. PCRE (MongoDB, MariaDB) and ICU (MySQL 8) also let it
// match before a final newline, so `"abc\n".matches("abc$")` is false in CEL
// and true on those servers. [EndOfText] replaces every such `$` with `\z`,
// which all three engines read as the absolute end, and leaves alone a `$`
// in multi-line mode (the same meaning everywhere), an escaped `\$`, and a
// `$` inside a character class or a `\Q…\E` quote.
//
//	pattern, err := regexanchor.EndOfText(`^A.*e$`) // `^A.*e\z`
//
// `^` needs no rewrite: without multi-line mode it is start-of-text in every
// engine.
package regexanchor
